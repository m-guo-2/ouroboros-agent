/**
 * JSONL 文件写入器
 * 
 * 按日志级别 + 日期写入不同文件：
 *   data/logs/boundary/2026-02-07.jsonl
 *   data/logs/business/2026-02-07.jsonl
 *   data/logs/detail/2026-02-07.jsonl
 * 
 * 写入策略：
 * - 内存缓冲，批量写入
 * - 每 500ms 或 buffer 超 50 条时 flush
 * - 进程退出时自动 flush
 * 
 * 控制台输出（环境变量控制）：
 * - LOG_CONSOLE=1          开启控制台输出（默认关闭）
 * - LOG_CONSOLE_LEVEL      最低输出级别：boundary | business | detail（默认 boundary，即全部输出）
 */

import { existsSync, mkdirSync, appendFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import type { LogLevel, LogEntry } from "./types";

/** 日志保留天数配置 */
const RETENTION_DAYS: Record<LogLevel, number> = {
  boundary: 30,
  business: 14,
  detail: 7,
};

/** 日志级别优先级（数值越小越重要） */
const LEVEL_PRIORITY: Record<LogLevel, number> = {
  boundary: 1,
  business: 2,
  detail: 3,
};

// ==================== 控制台格式化 ====================

/** ANSI 颜色码 */
const COLORS = {
  reset: "\x1b[0m",
  dim: "\x1b[2m",
  bold: "\x1b[1m",
  // 前景色
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  white: "\x1b[37m",
  gray: "\x1b[90m",
  // 背景色
  bgRed: "\x1b[41m",
  bgGreen: "\x1b[42m",
  bgYellow: "\x1b[43m",
  bgBlue: "\x1b[44m",
  bgMagenta: "\x1b[45m",
  bgCyan: "\x1b[46m",
};

/** 级别对应的颜色和标签 */
const LEVEL_STYLE: Record<LogLevel, { color: string; label: string }> = {
  boundary: { color: COLORS.cyan, label: "BND" },
  business: { color: COLORS.yellow, label: "BIZ" },
  detail: { color: COLORS.gray, label: "DTL" },
};

/** 状态对应的颜色 */
const STATUS_STYLE: Record<string, string> = {
  success: COLORS.green,
  error: COLORS.red,
  running: COLORS.blue,
};

/** 格式化一条日志为控制台可读字符串 */
function formatForConsole(level: LogLevel, entry: LogEntry): string {
  const style = LEVEL_STYLE[level];
  const time = entry.ts.slice(11, 23); // 'HH:mm:ss.SSS'
  const trace = entry.trace === "no-trace" ? "" : `${COLORS.dim}[${entry.trace}]${COLORS.reset} `;
  const statusColor = entry.status ? (STATUS_STYLE[entry.status] || "") : "";
  const statusTag = entry.status ? ` ${statusColor}${entry.status}${COLORS.reset}` : "";
  const op = `${COLORS.bold}${entry.op}${COLORS.reset}`;
  const service = entry.service !== "server" ? `${COLORS.dim}(${entry.service})${COLORS.reset} ` : "";

  let line = `${COLORS.dim}${time}${COLORS.reset} ${style.color}${style.label}${COLORS.reset} ${trace}${service}${op} ${entry.summary}${statusTag}`;

  // 附加 meta/data 的关键字段（紧凑显示）
  const extra = entry.meta || entry.data;
  if (extra) {
    const compact = compactMeta(extra);
    if (compact) {
      line += ` ${COLORS.dim}${compact}${COLORS.reset}`;
    }
  }

  return line;
}

/** 紧凑显示 meta/data 对象（只显示关键字段，避免刷屏） */
function compactMeta(obj: Record<string, unknown>, maxLen = 120): string {
  const parts: string[] = [];
  for (const [k, v] of Object.entries(obj)) {
    if (v === undefined || v === null) continue;
    // 跳过大字段
    if (typeof v === "string" && v.length > 200) {
      parts.push(`${k}=[${v.length} chars]`);
    } else if (typeof v === "object") {
      const s = JSON.stringify(v);
      parts.push(s.length > 80 ? `${k}={...}` : `${k}=${s}`);
    } else {
      parts.push(`${k}=${v}`);
    }
  }
  const result = parts.join(" ");
  return result.length > maxLen ? result.slice(0, maxLen) + "…" : result;
}

export class LogWriter {
  private baseDir: string;
  private buffers: Map<string, string[]> = new Map();
  private flushTimer: ReturnType<typeof setInterval> | null = null;
  private maxBufferSize = 50;
  private flushIntervalMs = 500;

  /** 控制台输出配置 */
  private consoleEnabled: boolean;
  private consoleMinLevel: number;

  constructor(baseDir: string) {
    this.baseDir = baseDir;

    // 从环境变量读取控制台输出配置
    this.consoleEnabled = process.env.LOG_CONSOLE === "1" || process.env.LOG_CONSOLE === "true";
    const minLevel = (process.env.LOG_CONSOLE_LEVEL || "boundary") as LogLevel;
    this.consoleMinLevel = LEVEL_PRIORITY[minLevel] || 1;

    this.ensureDirs();
    this.startFlushTimer();
    this.registerShutdownHook();
  }

  /** 写入一条日志 */
  write(level: LogLevel, entry: LogEntry): void {
    // 控制台输出（立即，不经过 buffer）
    if (this.consoleEnabled && LEVEL_PRIORITY[level] <= this.consoleMinLevel) {
      console.log(formatForConsole(level, entry));
    }

    const date = entry.ts.slice(0, 10); // '2026-02-07'
    const key = `${level}/${date}`;
    const line = JSON.stringify(entry);

    if (!this.buffers.has(key)) {
      this.buffers.set(key, []);
    }
    this.buffers.get(key)!.push(line);

    // buffer 满了立即 flush
    if (this.buffers.get(key)!.length >= this.maxBufferSize) {
      this.flushKey(key);
    }
  }

  /** 立即 flush 所有缓冲区 */
  flushAll(): void {
    for (const key of this.buffers.keys()) {
      this.flushKey(key);
    }
  }

  /** flush 指定 key 的缓冲区 */
  private flushKey(key: string): void {
    const lines = this.buffers.get(key);
    if (!lines || lines.length === 0) return;

    const content = lines.splice(0).join("\n") + "\n";
    const filePath = resolve(this.baseDir, `${key}.jsonl`);

    // 确保目录存在
    const dir = dirname(filePath);
    if (!existsSync(dir)) {
      mkdirSync(dir, { recursive: true });
    }

    try {
      appendFileSync(filePath, content, "utf-8");
    } catch (err) {
      // 日志写入失败不应影响业务
      console.error(`[LogWriter] Failed to write ${filePath}:`, err);
    }
  }

  /** 确保日志目录存在 */
  private ensureDirs(): void {
    const levels: LogLevel[] = ["boundary", "business", "detail"];
    for (const level of levels) {
      const dir = resolve(this.baseDir, level);
      if (!existsSync(dir)) {
        mkdirSync(dir, { recursive: true });
      }
    }
  }

  /** 启动定时 flush */
  private startFlushTimer(): void {
    this.flushTimer = setInterval(() => {
      this.flushAll();
    }, this.flushIntervalMs);
  }

  /** 注册进程退出钩子，确保日志不丢 */
  private registerShutdownHook(): void {
    const flush = () => {
      this.flushAll();
      if (this.flushTimer) {
        clearInterval(this.flushTimer);
        this.flushTimer = null;
      }
    };

    process.on("beforeExit", flush);
    process.on("SIGINT", flush);
    process.on("SIGTERM", flush);
  }

  /**
   * 清理过期日志文件
   * 根据 RETENTION_DAYS 配置删除过期的 .jsonl 文件
   */
  async cleanup(): Promise<{ deleted: string[]; errors: string[] }> {
    const { readdirSync, unlinkSync } = await import("node:fs");
    const deleted: string[] = [];
    const errors: string[] = [];

    const now = new Date();

    for (const [level, days] of Object.entries(RETENTION_DAYS)) {
      const dir = resolve(this.baseDir, level);
      if (!existsSync(dir)) continue;

      const cutoff = new Date(now.getTime() - days * 24 * 60 * 60 * 1000);
      const cutoffDate = cutoff.toISOString().slice(0, 10);

      try {
        const files = readdirSync(dir);
        for (const file of files) {
          if (!file.endsWith(".jsonl")) continue;
          const fileDate = file.replace(".jsonl", "");
          if (fileDate < cutoffDate) {
            try {
              unlinkSync(resolve(dir, file));
              deleted.push(`${level}/${file}`);
            } catch (err) {
              errors.push(`${level}/${file}: ${err}`);
            }
          }
        }
      } catch (err) {
        errors.push(`${level}/: ${err}`);
      }
    }

    return { deleted, errors };
  }
}
