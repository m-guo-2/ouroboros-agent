/**
 * Tool Registry — 统一工具注册中心
 *
 * 聚合三类工具来源：
 *   1. 内置工具 (builtin): send_channel_message, get_skill_doc 等
 *   2. Skill 工具 (skill): 从 skill-manager 加载，按 executor 类型路由执行
 *   3. MCP 工具 (mcp): 通过 HTTP 从远端 MCP Server 拉取并代理执行
 *
 * 对 Agent Loop 暴露统一的 RegisteredTool[] 接口，
 * 引擎不需要关心工具来自哪里。
 */

import type {
  RegisteredTool,
  ToolDefinition,
  ToolExecutor,
} from "./types";
import type {
  SkillContext,
  SkillToolExecutor,
} from "../services/server-client";

// ==================== MCP Server 配置 ====================

export interface McpServerConfig {
  name: string;
  baseUrl: string;
  apiKey?: string;
}

interface McpToolsResponse {
  tools: Array<{
    name: string;
    description: string;
    inputSchema: {
      type: "object";
      properties: Record<string, unknown>;
      required?: string[];
    };
  }>;
}

// ==================== 工具执行器工厂 ====================

function createSkillHttpExecutor(executor: SkillToolExecutor): ToolExecutor {
  return async (input) => {
    if (!executor.url) throw new Error("HTTP executor missing url");

    const method = (executor.method || "POST").toUpperCase();
    const response = await fetch(executor.url, {
      method,
      headers: { "Content-Type": "application/json" },
      body: method === "GET" ? undefined : JSON.stringify(input),
    });

    const text = await response.text();
    if (!response.ok) {
      throw new Error(`HTTP tool failed: ${response.status} ${text}`);
    }

    try {
      return JSON.parse(text);
    } catch {
      return text;
    }
  };
}

function createMcpToolExecutor(
  serverConfig: McpServerConfig,
  toolName: string,
): ToolExecutor {
  return async (input) => {
    const url = `${serverConfig.baseUrl}/tools/${toolName}/call`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (serverConfig.apiKey) {
      headers["Authorization"] = `Bearer ${serverConfig.apiKey}`;
    }

    const response = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify({ arguments: input }),
    });

    const text = await response.text();
    if (!response.ok) {
      throw new Error(`MCP tool ${toolName} failed: ${response.status} ${text}`);
    }

    try {
      const json = JSON.parse(text);
      return json.content ?? json.result ?? json;
    } catch {
      return text;
    }
  };
}

// ==================== Tool Registry ====================

export class ToolRegistry {
  private tools = new Map<string, RegisteredTool>();

  /** 当前已注册的所有工具 */
  getAll(): RegisteredTool[] {
    return Array.from(this.tools.values());
  }

  /** 按名称查找工具 */
  get(name: string): RegisteredTool | undefined {
    return this.tools.get(name);
  }

  /** 获取所有工具的定义（传给 LLM） */
  getDefinitions(): ToolDefinition[] {
    return this.getAll().map((t) => t.definition);
  }

  /** 执行工具 */
  async execute(name: string, input: Record<string, unknown>): Promise<unknown> {
    const tool = this.tools.get(name);
    if (!tool) {
      throw new Error(`Tool not found: ${name}`);
    }
    return tool.execute(input);
  }

  has(name: string): boolean {
    return this.tools.has(name);
  }

  // ==================== 注册：内置工具 ====================

  registerBuiltin(
    name: string,
    description: string,
    inputSchema: ToolDefinition["input_schema"],
    executor: ToolExecutor,
  ): void {
    this.tools.set(name, {
      definition: { name, description, input_schema: inputSchema },
      execute: executor,
      source: "builtin",
      sourceName: "system",
    });
  }

  // ==================== 注册：Skill 工具 ====================

  /**
   * 从 SkillContext 批量注册 Skill 工具。
   * 需要额外传入 internalHandlers 来处理 type=internal 的工具。
   */
  registerSkills(
    skillsCtx: SkillContext,
    internalHandlers: Record<string, ToolExecutor>,
  ): void {
    for (const toolDef of skillsCtx.tools) {
      const executor = skillsCtx.toolExecutors[toolDef.name];
      if (!executor) continue;

      let execute: ToolExecutor;

      if (executor.type === "http") {
        execute = createSkillHttpExecutor(executor);
      } else if (executor.type === "internal") {
        const handler = internalHandlers[executor.handler || toolDef.name];
        if (!handler) {
          console.warn(`[tool-registry] No internal handler for: ${executor.handler || toolDef.name}`);
          continue;
        }
        execute = handler;
      } else {
        console.warn(`[tool-registry] Unsupported executor type: ${executor.type}`);
        continue;
      }

      this.tools.set(toolDef.name, {
        definition: {
          name: toolDef.name,
          description: toolDef.description,
          input_schema: toolDef.input_schema,
        },
        execute,
        source: "skill",
        sourceName: toolDef.description.match(/\[Skill: (.+?)\]/)?.[1] || "unknown",
      });
    }
  }

  // ==================== 注册：MCP 工具 ====================

  /**
   * 从 MCP Server 拉取工具列表并注册。
   * 失败时不抛异常，仅打印警告（MCP 不可用不应阻塞 Agent）。
   */
  async registerMcpServer(config: McpServerConfig): Promise<number> {
    try {
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
      };
      if (config.apiKey) {
        headers["Authorization"] = `Bearer ${config.apiKey}`;
      }

      const response = await fetch(`${config.baseUrl}/tools/list`, {
        method: "POST",
        headers,
        body: JSON.stringify({}),
      });

      if (!response.ok) {
        console.warn(`[tool-registry] MCP ${config.name} tools/list failed: ${response.status}`);
        return 0;
      }

      const data = await response.json() as McpToolsResponse;
      const tools = data.tools || [];

      for (const tool of tools) {
        const name = `mcp_${config.name}_${tool.name}`;
        this.tools.set(name, {
          definition: {
            name,
            description: `[MCP: ${config.name}] ${tool.description}`,
            input_schema: tool.inputSchema,
          },
          execute: createMcpToolExecutor(config, tool.name),
          source: "mcp",
          sourceName: config.name,
        });
      }

      return tools.length;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      console.warn(`[tool-registry] MCP ${config.name} discovery failed: ${msg}`);
      return 0;
    }
  }

  /** 清空所有工具（Session 重建时用） */
  clear(): void {
    this.tools.clear();
  }
}
