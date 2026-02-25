/**
 * 服务管理器
 * 从 Admin 管理子服务（agent、channel-feishu、channel-qiwei）的启停
 * 支持 SSE 实时日志推送 + 文件日志持久化
 *
 * 控制台镜像开关：
 * - SERVICE_LOG_CONSOLE=1            开启子服务日志镜像到 server 控制台
 * - SERVICE_LOG_CONSOLE_STDOUT=0     关闭 stdout 镜像（默认开启）
 * - SERVICE_LOG_CONSOLE_STDERR=0     关闭 stderr 镜像（默认开启）
 * - SERVICE_LOG_SESSION_ID=<id>      仅输出指定 sessionId 的日志（精确匹配+包含匹配兜底）
 * - SERVICE_LOG_TRACE_ID=<id>        仅输出指定 traceId 的日志（精确匹配+包含匹配兜底）
 * - SERVICE_LOG_COLOR=0              关闭 ANSI 颜色高亮
 */

import { resolve } from "path";
import { spawn, type ChildProcess } from "child_process";
import { EventEmitter } from "events";
import { mkdirSync, existsSync, appendFileSync, readFileSync } from "fs";
import { createConnection, type Socket } from "net";
import { settingsDb } from "./database";

const PROJECT_ROOT = resolve(import.meta.dir, "../../..");
const LOGS_DIR = resolve(import.meta.dir, "../../data/logs/services");

// 确保日志目录存在
if (!existsSync(LOGS_DIR)) {
  mkdirSync(LOGS_DIR, { recursive: true });
}

export interface ServiceInfo {
  name: string;
  label: string;
  description: string;
  cwd: string;
  command: string;
  args: string[];
  defaultPort: number;
  portSettingKey?: string;
  status: "stopped" | "running" | "starting" | "error";
  pid?: number;
  startedAt?: number;
  error?: string;
  logs: string[];
  logFile?: string; // 当前日志文件路径
  portAlive?: boolean; // 端口探测结果（用于检测外部启动的服务）
  externalProcess?: boolean; // 是否由外部进程管理
}

/** 实时日志行 */
export interface LogLine {
  timestamp: number;
  content: string;
  stream: "stdout" | "stderr";
}

const MAX_LOG_LINES = 500;

class ServiceManager extends EventEmitter {
  private services: Map<string, ServiceInfo> = new Map();
  private processes: Map<string, ChildProcess> = new Map();
  private healthCheckInterval?: ReturnType<typeof setInterval>;
  private readonly consoleMirrorEnabled: boolean;
  private readonly consoleMirrorIncludeStdout: boolean;
  private readonly consoleMirrorIncludeStderr: boolean;
  private readonly consoleFilterSessionId?: string;
  private readonly consoleFilterTraceId?: string;
  private readonly consoleColorEnabled: boolean;

  constructor() {
    super();
    this.setMaxListeners(50); // 支持多个 SSE 客户端同时监听
    const serviceLogConsole = process.env.SERVICE_LOG_CONSOLE;
    // 默认开启子服务日志镜像，便于联调排障；可通过 SERVICE_LOG_CONSOLE=0 显式关闭
    if (serviceLogConsole === undefined) {
      this.consoleMirrorEnabled = true;
    } else {
      this.consoleMirrorEnabled = serviceLogConsole === "1" || serviceLogConsole === "true";
    }
    this.consoleMirrorIncludeStdout =
      process.env.SERVICE_LOG_CONSOLE_STDOUT !== "0" &&
      process.env.SERVICE_LOG_CONSOLE_STDOUT !== "false";
    this.consoleMirrorIncludeStderr =
      process.env.SERVICE_LOG_CONSOLE_STDERR !== "0" &&
      process.env.SERVICE_LOG_CONSOLE_STDERR !== "false";
    const rawSessionFilter = (process.env.SERVICE_LOG_SESSION_ID || "").trim();
    const rawTraceFilter = (process.env.SERVICE_LOG_TRACE_ID || "").trim();
    this.consoleFilterSessionId = rawSessionFilter || undefined;
    this.consoleFilterTraceId = rawTraceFilter || undefined;
    this.consoleColorEnabled =
      process.env.SERVICE_LOG_COLOR !== "0" &&
      process.env.SERVICE_LOG_COLOR !== "false" &&
      process.env.NO_COLOR !== "1";
    this.registerServices();
    this.startHealthCheck();
  }

  private registerServices() {
    this.services.set("agent", {
      name: "agent",
      label: "Agent 执行引擎",
      description: "AI Agent 执行引擎，围绕 Claude Agent SDK 构建",
      cwd: resolve(PROJECT_ROOT, "agent"),
      command: "bun",
      args: ["run", "src/index.ts"],
      defaultPort: 1996,
      portSettingKey: "general.orchestrator_port",
      status: "stopped",
      logs: [],
    });

    this.services.set("channel-feishu", {
      name: "channel-feishu",
      label: "飞书渠道",
      description: "飞书渠道适配器，接收消息并与 Agent 联动",
      cwd: resolve(PROJECT_ROOT, "channel-feishu"),
      command: "bun",
      args: ["run", "src/index.ts"],
      defaultPort: 1999,
      portSettingKey: "general.feishu_port",
      status: "stopped",
      logs: [],
    });

    this.services.set("channel-qiwei", {
      name: "channel-qiwei",
      label: "企微渠道",
      description: "企微渠道适配器，接收消息并与 Agent 联动",
      cwd: resolve(PROJECT_ROOT, "channel-qiwei"),
      command: "bun",
      args: ["run", "src/index.ts"],
      defaultPort: 2000,
      portSettingKey: "general.qiwei_port",
      status: "stopped",
      logs: [],
    });
  }

  /**
   * 获取所有服务状态（同步返回，后台异步探测端口）
   */
  getAllStatus(): ServiceInfo[] {
    return Array.from(this.services.values()).map((s) => ({
      ...s,
      // 如果本进程没有管理它，但 portAlive 为 true，说明是外部启动的
      status: s.status === "stopped" && s.portAlive ? "running" as const : s.status,
      externalProcess: s.status === "stopped" && s.portAlive ? true : undefined,
      logs: [], // 不返回完整日志
    }));
  }

  /**
   * 获取单个服务状态
   */
  getStatus(name: string): ServiceInfo | null {
    const s = this.services.get(name);
    if (!s) return null;
    return {
      ...s,
      status: s.status === "stopped" && s.portAlive ? "running" as const : s.status,
    };
  }

  /**
   * 异步探测端口是否可达（TCP connect 探针）
   */
  private probePort(port: number, timeout = 1000): Promise<boolean> {
    return new Promise((resolve) => {
      const socket: Socket = createConnection({ port, host: "127.0.0.1" }, () => {
        socket.destroy();
        resolve(true);
      });
      socket.setTimeout(timeout);
      socket.on("timeout", () => { socket.destroy(); resolve(false); });
      socket.on("error", () => { socket.destroy(); resolve(false); });
    });
  }

  /**
   * 定期探测所有服务端口（后台运行）
   */
  private startHealthCheck() {
    // 启动后立即探测一次
    this.probeAllPorts();
    // 每 5 秒探测一次
    this.healthCheckInterval = setInterval(() => this.probeAllPorts(), 5000);
  }

  private async probeAllPorts() {
    for (const [name, service] of this.services) {
      // 如果是本进程管理的运行中服务，跳过探测
      if (this.processes.has(name)) {
        service.portAlive = true;
        continue;
      }
      const port = this.getServicePort(name);
      service.portAlive = await this.probePort(port);
    }
  }

  /**
   * 获取服务实际使用的端口（优先从 settings 读取）
   */
  private getServicePort(name: string): number {
    const service = this.services.get(name);
    if (!service) return 0;
    if (service.portSettingKey) {
      const customPort = settingsDb.get(service.portSettingKey);
      if (customPort) return parseInt(customPort, 10) || service.defaultPort;
    }
    return service.defaultPort;
  }

  /**
   * 获取服务日志
   */
  getLogs(name: string, lines = 50): string[] {
    const service = this.services.get(name);
    if (!service) return [];
    return service.logs.slice(-lines);
  }

  /**
   * 启动服务
   */
  async start(name: string): Promise<{ success: boolean; error?: string }> {
    const service = this.services.get(name);
    if (!service) {
      return { success: false, error: `服务 ${name} 不存在` };
    }

    if (service.status === "running") {
      return { success: false, error: `服务 ${name} 已在运行` };
    }

    // 检查必要配置
    const configCheck = this.checkServiceConfig(name);
    if (!configCheck.ok) {
      return { success: false, error: configCheck.error };
    }

    service.status = "starting";
    service.logs = [];
    service.error = undefined;

    // 创建日志文件
    const logFile = this.createLogFile(name);
    service.logFile = logFile;

    try {
      // 构建环境变量：从 settings DB 读取并注入
      const env = this.buildServiceEnv(name);

      const child = spawn(service.command, service.args, {
        cwd: service.cwd,
        env: { ...process.env, ...env },
        stdio: ["ignore", "pipe", "pipe"],
      });

      this.processes.set(name, child);
      service.pid = child.pid;
      service.startedAt = Date.now();

      // 写入启动头
      this.appendToLogFile(logFile, `\n${"=".repeat(60)}\n[${new Date().toISOString()}] 服务启动: ${name} (PID: ${child.pid})\n${"=".repeat(60)}\n`);

      // 收集日志 + 实时推送 + 文件持久化
      child.stdout?.on("data", (data: Buffer) => {
        const lines = data.toString().split("\n").filter(Boolean);
        const now = Date.now();
        const ts = new Date(now).toISOString();
        for (const line of lines) {
          service.logs.push(line);
          this.emit(`log:${name}`, { timestamp: now, content: line, stream: "stdout" } as LogLine);
          this.appendToLogFile(logFile, `[${ts}] ${line}\n`);
          this.mirrorToConsole(name, "stdout", line);
        }
        if (service.logs.length > MAX_LOG_LINES) {
          service.logs = service.logs.slice(-MAX_LOG_LINES);
        }
      });

      child.stderr?.on("data", (data: Buffer) => {
        const lines = data.toString().split("\n").filter(Boolean);
        const now = Date.now();
        const ts = new Date(now).toISOString();
        for (const line of lines) {
          const content = `[stderr] ${line}`;
          service.logs.push(content);
          this.emit(`log:${name}`, { timestamp: now, content, stream: "stderr" } as LogLine);
          this.appendToLogFile(logFile, `[${ts}] [stderr] ${line}\n`);
          this.mirrorToConsole(name, "stderr", line);
        }
        if (service.logs.length > MAX_LOG_LINES) {
          service.logs = service.logs.slice(-MAX_LOG_LINES);
        }
      });

      child.on("error", (err) => {
        service.status = "error";
        service.error = err.message;
        this.processes.delete(name);
      });

      child.on("exit", (code) => {
        service.status = "stopped";
        service.pid = undefined;
        if (code !== 0 && code !== null) {
          service.error = `进程退出，代码: ${code}`;
        }
        this.appendToLogFile(logFile, `[${new Date().toISOString()}] 服务退出: ${name} (code: ${code})\n`);
        this.processes.delete(name);
      });

      // 等一下确认进程启动
      await new Promise((resolve) => setTimeout(resolve, 1000));

      if (service.status !== "error") {
        service.status = "running";
      }

      return { success: service.status === "running" };
    } catch (err) {
      service.status = "error";
      service.error = String(err);
      return { success: false, error: String(err) };
    }
  }

  /**
   * 停止服务
   */
  async stop(name: string): Promise<{ success: boolean; error?: string }> {
    const service = this.services.get(name);
    if (!service) {
      return { success: false, error: `服务 ${name} 不存在` };
    }

    const child = this.processes.get(name);
    if (!child) {
      service.status = "stopped";
      return { success: true };
    }

    return new Promise((resolve) => {
      const timeout = setTimeout(() => {
        child.kill("SIGKILL");
        resolve({ success: true });
      }, 5000);

      child.on("exit", () => {
        clearTimeout(timeout);
        service.status = "stopped";
        service.pid = undefined;
        this.processes.delete(name);
        resolve({ success: true });
      });

      child.kill("SIGTERM");
    });
  }

  /**
   * 重启服务
   */
  async restart(name: string): Promise<{ success: boolean; error?: string }> {
    await this.stop(name);
    // 等待进程完全退出
    await new Promise((r) => setTimeout(r, 500));
    return this.start(name);
  }

  /**
   * 停止所有服务
   */
  async shutdown(): Promise<void> {
    if (this.healthCheckInterval) {
      clearInterval(this.healthCheckInterval);
    }
    const names = Array.from(this.processes.keys());
    await Promise.all(names.map((n) => this.stop(n)));
  }

  /**
   * 检查服务所需配置
   */
  private checkServiceConfig(name: string): { ok: boolean; error?: string } {
    if (name === "channel-feishu") {
      const appId = settingsDb.get("feishu.app_id");
      const appSecret = settingsDb.get("feishu.app_secret");
      if (!appId || !appSecret) {
        return { ok: false, error: "请先配置飞书 App ID 和 App Secret" };
      }
    }

    if (name === "channel-qiwei") {
      const token = settingsDb.get("qiwei.token");
      const guid = settingsDb.get("qiwei.guid");
      if (!token || !guid) {
        return { ok: false, error: "请先配置企微 Token 和 GUID" };
      }
    }

    if (name === "agent") {
      // 根据选择的 provider 检查对应的 API Key
      const provider = settingsDb.get("orchestrator.provider") || "anthropic";
      const keyMap: Record<string, string> = {
        anthropic: "api_key.anthropic",
        moonshot: "api_key.moonshot",
        openai: "api_key.openai",
        zhipu: "api_key.zhipu",
      };
      const settingKey = keyMap[provider] || "api_key.anthropic";
      const apiKey = settingsDb.get(settingKey);
      if (!apiKey) {
        // 兜底：检查是否有任意一个 API key
        const anyKey = Object.values(keyMap).some(k => !!settingsDb.get(k));
        if (!anyKey) {
          return { ok: false, error: `请先配置 LLM API Key（当前 provider: ${provider}，需要 ${settingKey}）` };
        }
      }
    }

    return { ok: true };
  }

  // ==================== 日志文件操作 ====================

  /**
   * 创建日志文件路径（按日期 + 服务名）
   */
  private createLogFile(name: string): string {
    const date = new Date().toISOString().split("T")[0]; // YYYY-MM-DD
    const logDir = resolve(LOGS_DIR, name);
    if (!existsSync(logDir)) {
      mkdirSync(logDir, { recursive: true });
    }
    return resolve(logDir, `${date}.log`);
  }

  /**
   * 追加写入日志文件
   */
  private appendToLogFile(filePath: string, content: string): void {
    try {
      appendFileSync(filePath, content, "utf-8");
    } catch {
      // 日志写入失败不应影响服务运行
    }
  }

  /**
   * 子服务日志镜像到 server 控制台，便于本地联调排障
   */
  private mirrorToConsole(name: string, stream: "stdout" | "stderr", line: string): void {
    if (!this.consoleMirrorEnabled) return;
    if (stream === "stdout" && !this.consoleMirrorIncludeStdout) return;
    if (stream === "stderr" && !this.consoleMirrorIncludeStderr) return;

    const parsed = this.extractLogIdentity(line);
    if (!this.shouldMirrorLine(line, parsed)) return;

    const ts = new Date().toISOString();
    const coloredService = this.colorize(name, "cyan");
    const coloredStream = stream === "stderr"
      ? this.colorize(stream, "red")
      : this.colorize(stream, "gray");
    const tags = [`[service:${coloredService}]`, `[${coloredStream}]`];
    if (parsed.sessionId) {
      tags.push(`[session:${this.colorize(parsed.sessionId, "yellow")}]`);
    }
    if (parsed.traceId) {
      tags.push(`[trace:${this.colorize(parsed.traceId, "magenta")}]`);
    }
    const prefix = tags.join("");

    if (stream === "stderr") {
      console.error(`${ts} ${prefix} ${line}`);
      return;
    }
    console.log(`${ts} ${prefix} ${line}`);
  }

  private colorize(value: string, color: "gray" | "red" | "yellow" | "cyan" | "magenta"): string {
    if (!this.consoleColorEnabled || !process.stdout.isTTY) return value;
    const codeMap: Record<typeof color, string> = {
      gray: "\u001b[90m",
      red: "\u001b[31m",
      yellow: "\u001b[33m",
      cyan: "\u001b[36m",
      magenta: "\u001b[35m",
    };
    const reset = "\u001b[0m";
    return `${codeMap[color]}${value}${reset}`;
  }

  private shouldMirrorLine(line: string, parsed: { sessionId?: string; traceId?: string }): boolean {
    const sessionFilter = this.consoleFilterSessionId;
    const traceFilter = this.consoleFilterTraceId;

    if (!sessionFilter && !traceFilter) return true;

    const sessionMatched = !sessionFilter
      || parsed.sessionId === sessionFilter
      || line.includes(sessionFilter);
    const traceMatched = !traceFilter
      || parsed.traceId === traceFilter
      || line.includes(traceFilter);

    return sessionMatched && traceMatched;
  }

  private extractLogIdentity(line: string): { sessionId?: string; traceId?: string } {
    const directSession = this.findFirstMatch(line, [
      /\bsessionId[=:]\s*([A-Za-z0-9._:-]+)/i,
      /\bsession_id[=:]\s*([A-Za-z0-9._:-]+)/i,
      /\bsession[=:]\s*([A-Za-z0-9._:-]+)/i,
      /\bsession[^(]*\(([a-f0-9]{8}-[a-f0-9-]{27,})\)/i,
      /\bsession\s+([a-f0-9]{8}-[a-f0-9-]{27,})/i,
      /\/\.agent-sessions\/([A-Za-z0-9._:-]+)/i,
    ]);
    const directTrace = this.findFirstMatch(line, [
      /\btraceId[=:]\s*([A-Za-z0-9._:-]+)/i,
      /\btrace_id[=:]\s*([A-Za-z0-9._:-]+)/i,
      /\btrace[=:]\s*([A-Za-z0-9._:-]+)/i,
    ]);

    const json = this.extractJsonObject(line);
    const jsonSession = this.pickFirstString(json, [
      ["sessionId"],
      ["session_id"],
      ["session"],
      ["meta", "sessionId"],
      ["meta", "session_id"],
      ["data", "sessionId"],
      ["data", "session_id"],
    ]);
    const jsonTrace = this.pickFirstString(json, [
      ["traceId"],
      ["trace_id"],
      ["trace"],
      ["meta", "traceId"],
      ["meta", "trace_id"],
      ["data", "traceId"],
      ["data", "trace_id"],
    ]);

    return {
      sessionId: directSession || jsonSession,
      traceId: directTrace || jsonTrace,
    };
  }

  private findFirstMatch(line: string, patterns: RegExp[]): string | undefined {
    for (const pattern of patterns) {
      const match = line.match(pattern);
      if (match && match[1]) {
        return match[1];
      }
    }
    return undefined;
  }

  private extractJsonObject(line: string): Record<string, unknown> | null {
    const trimmed = line.trim();
    if (!trimmed.startsWith("{") || !trimmed.endsWith("}")) {
      return null;
    }
    try {
      const parsed = JSON.parse(trimmed);
      return parsed && typeof parsed === "object" ? parsed as Record<string, unknown> : null;
    } catch {
      return null;
    }
  }

  private pickFirstString(
    root: Record<string, unknown> | null,
    paths: string[][],
  ): string | undefined {
    if (!root) return undefined;
    for (const path of paths) {
      let cur: unknown = root;
      let hit = true;
      for (const key of path) {
        if (!cur || typeof cur !== "object" || !(key in (cur as Record<string, unknown>))) {
          hit = false;
          break;
        }
        cur = (cur as Record<string, unknown>)[key];
      }
      if (hit && typeof cur === "string" && cur.trim()) {
        return cur.trim();
      }
    }
    return undefined;
  }

  /**
   * 获取服务的日志文件路径
   */
  getLogFilePath(name: string): string | null {
    const service = this.services.get(name);
    return service?.logFile || null;
  }

  /**
   * 读取日志文件的最后 N 行
   */
  readLogFile(name: string, tailLines = 100): { path: string | null; lines: string[] } {
    const service = this.services.get(name);
    const logFile = service?.logFile;

    if (!logFile || !existsSync(logFile)) {
      return { path: logFile || null, lines: [] };
    }

    try {
      const content = readFileSync(logFile, "utf-8");
      const allLines = content.split("\n").filter(Boolean);
      return {
        path: logFile,
        lines: allLines.slice(-tailLines),
      };
    } catch {
      return { path: logFile, lines: [] };
    }
  }

  /**
   * 从 settings DB 构建服务环境变量
   */
  private buildServiceEnv(name: string): Record<string, string> {
    const env: Record<string, string> = {};

    // 通用 API Keys
    const anthropicKey = settingsDb.get("api_key.anthropic");
    const openaiKey = settingsDb.get("api_key.openai");
    const moonshotKey = settingsDb.get("api_key.moonshot");
    const zhipuKey = settingsDb.get("api_key.zhipu");

    if (anthropicKey) env.ANTHROPIC_API_KEY = anthropicKey;
    if (openaiKey) env.OPENAI_API_KEY = openaiKey;
    if (moonshotKey) env.MOONSHOT_API_KEY = moonshotKey;
    if (zhipuKey) env.ZHIPU_API_KEY = zhipuKey;

    // Base URLs
    const openaiBaseUrl = settingsDb.get("base_url.openai");
    const moonshotBaseUrl = settingsDb.get("base_url.moonshot");
    const zhipuBaseUrl = settingsDb.get("base_url.zhipu");

    if (openaiBaseUrl) env.OPENAI_BASE_URL = openaiBaseUrl;
    if (moonshotBaseUrl) env.MOONSHOT_BASE_URL = moonshotBaseUrl;
    if (zhipuBaseUrl) env.ZHIPU_BASE_URL = zhipuBaseUrl;

    // 服务特定环境变量
    if (name === "channel-feishu") {
      const appId = settingsDb.get("feishu.app_id");
      const appSecret = settingsDb.get("feishu.app_secret");
      const encryptKey = settingsDb.get("feishu.encrypt_key");
      const verificationToken = settingsDb.get("feishu.verification_token");
      const port = settingsDb.get("general.feishu_port");

      if (appId) env.FEISHU_APP_ID = appId;
      if (appSecret) env.FEISHU_APP_SECRET = appSecret;
      if (encryptKey) env.FEISHU_ENCRYPT_KEY = encryptKey;
      if (verificationToken) env.FEISHU_VERIFICATION_TOKEN = verificationToken;
      if (port) env.FEISHU_BOT_PORT = port;
    }

    if (name === "channel-qiwei") {
      const token = settingsDb.get("qiwei.token");
      const guid = settingsDb.get("qiwei.guid");
      const apiBaseUrl = settingsDb.get("qiwei.api_base_url");
      const port = settingsDb.get("general.qiwei_port");

      if (token) env.QIWEI_TOKEN = token;
      if (guid) env.QIWEI_GUID = guid;
      if (apiBaseUrl) env.QIWEI_API_BASE_URL = apiBaseUrl;
      if (port) env.QIWEI_BOT_PORT = port;
    }

    if (name === "agent") {
      const port = settingsDb.get("general.orchestrator_port");
      if (port) env.ORCHESTRATOR_PORT = port;

      // 根据 provider 映射 API key 和 base URL 给 Orchestrator
      const provider = settingsDb.get("orchestrator.provider") || "anthropic";
      const model = settingsDb.get("orchestrator.model") || "";

      const keyMap: Record<string, string> = {
        anthropic: "api_key.anthropic",
        moonshot: "api_key.moonshot",
        openai: "api_key.openai",
        zhipu: "api_key.zhipu",
      };
      const urlMap: Record<string, string> = {
        anthropic: "base_url.anthropic",
        moonshot: "base_url.moonshot",
        openai: "base_url.openai",
        zhipu: "base_url.zhipu",
      };

      const apiKey = settingsDb.get(keyMap[provider] || "api_key.anthropic") || "";
      const baseUrl = settingsDb.get(urlMap[provider] || "base_url.anthropic") || "";

      // Anthropic 兼容提供商（原生 Anthropic、Moonshot 等）：SDK 可直连，跳过 Proxy
      const anthropicCompatProviders = ["anthropic", "moonshot"];
      if (anthropicCompatProviders.includes(provider) && apiKey) {
        // Claude Agent SDK / Claude Code 使用 ANTHROPIC_AUTH_TOKEN 做认证
        // 同时设置 ANTHROPIC_API_KEY 作为兜底
        env.ANTHROPIC_API_KEY = apiKey;
        env.ANTHROPIC_AUTH_TOKEN = apiKey;
        if (baseUrl) {
          env.ANTHROPIC_BASE_URL = baseUrl;
        }
        env.ORCHESTRATOR_DIRECT_MODE = "true";

        // 参照 Moonshot 官方文档：需要覆盖所有模型别名环境变量
        // 否则 Claude Code 会使用默认的 Claude 模型名
        if (model) {
          env.ANTHROPIC_MODEL = model;
          env.ANTHROPIC_DEFAULT_OPUS_MODEL = model;
          env.ANTHROPIC_DEFAULT_SONNET_MODEL = model;
          env.ANTHROPIC_DEFAULT_HAIKU_MODEL = model;
          env.CLAUDE_CODE_SUBAGENT_MODEL = model;
        }
      }

      // OpenAI 兼容提供商：走 Proxy（Anthropic → OpenAI 格式转换）
      if (!anthropicCompatProviders.includes(provider) && apiKey) {
        env.PROXY_TARGET_KEY = apiKey;
        if (baseUrl) env.PROXY_TARGET_URL = baseUrl;
      }

      if (model) env.PROXY_TARGET_MODEL = model;
    }

    return env;
  }
}

export const serviceManager = new ServiceManager();

// 优雅关闭
process.on("SIGINT", async () => {
  await serviceManager.shutdown();
});
process.on("SIGTERM", async () => {
  await serviceManager.shutdown();
});
