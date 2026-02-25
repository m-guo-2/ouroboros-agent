/**
 * 观测事件总线
 * 
 * 基于 EventEmitter 的内存事件总线，用于将 Agent 执行过程中的事件
 * 实时推送到 Monitor 前端（通过 SSE）。
 * 
 * channel-dispatcher 处理消息时，将执行事件推送到此总线；
 * SSE 端点监听总线事件并转发给前端。
 */

import { EventEmitter } from "node:events";

/**
 * 观测事件类型
 */
export interface ObservationEvent {
  /** 事件类型 */
  type: "execution_start" | "thinking" | "reasoning" | "tool_call" | "tool_result" | "decision_step" | "execution_done" | "error";
  /** 所属会话 ID */
  sessionId: string;
  /** Agent ID */
  agentId?: string;
  /** 用户 ID */
  userId?: string;
  /** 来源渠道 */
  channel?: string;
  /** 关联的 traceId */
  traceId?: string;
  /** 消息发起方：user（用户发消息触发）| agent（Agent 主动行动）| system（系统/定时任务） */
  initiator?: "user" | "agent" | "system";
  /** 事件时间戳 */
  timestamp: number;
  /** 事件数据（不同类型有不同结构） */
  data?: Record<string, unknown>;
}

/**
 * 观测事件过滤条件
 */
export interface ObservationFilter {
  agentId?: string;
  channel?: string;
  sessionId?: string;
}

class ObservationBus {
  private emitter = new EventEmitter();
  
  constructor() {
    // 设置较大的监听器上限（多个 SSE 连接）
    this.emitter.setMaxListeners(50);
  }

  /**
   * 发送观测事件
   */
  emit(event: ObservationEvent): void {
    this.emitter.emit("observation", event);
  }

  /**
   * 订阅观测事件
   * 返回取消订阅的函数
   */
  subscribe(
    callback: (event: ObservationEvent) => void,
    filter?: ObservationFilter
  ): () => void {
    const handler = (event: ObservationEvent) => {
      // 应用过滤条件
      if (filter) {
        if (filter.agentId && event.agentId !== filter.agentId) return;
        if (filter.channel && event.channel !== filter.channel) return;
        if (filter.sessionId && event.sessionId !== filter.sessionId) return;
      }
      callback(event);
    };

    this.emitter.on("observation", handler);

    return () => {
      this.emitter.off("observation", handler);
    };
  }

  /**
   * 获取当前监听器数量
   */
  listenerCount(): number {
    return this.emitter.listenerCount("observation");
  }
}

/** 全局单例 */
export const observationBus = new ObservationBus();
