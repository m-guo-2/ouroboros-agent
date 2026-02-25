/**
 * JSONL 日志读取器
 * 
 * 从 data/logs/{level}/{date}.jsonl 文件中读取和查询日志条目。
 * 支持按 traceId、spanId 过滤，以及反向读取最近的日志。
 */

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import type { LogLevel, LogEntry } from "./types";

/** 默认日志目录 */
const DEFAULT_LOG_DIR = resolve(import.meta.dir, "../../../../data/logs");

export interface LogQueryOptions {
  /** 按 traceId 过滤 */
  traceId?: string;
  /** 按 spanId 过滤 */
  spanId?: string;
  /** 日志级别过滤 */
  level?: LogLevel | LogLevel[];
  /** 操作类型过滤 */
  op?: string | string[];
  /** 最大结果数 */
  limit?: number;
  /** 查询的日期范围（ISO 日期字符串，如 '2026-02-08'）*/
  dateFrom?: string;
  dateTo?: string;
}

/**
 * 读取指定日期和级别的 JSONL 文件
 */
function readJsonlFile(logDir: string, level: LogLevel, date: string): LogEntry[] {
  const filePath = resolve(logDir, level, `${date}.jsonl`);
  if (!existsSync(filePath)) return [];

  try {
    const content = readFileSync(filePath, "utf-8");
    const lines = content.trim().split("\n").filter(Boolean);
    return lines.map((line) => {
      try {
        return JSON.parse(line) as LogEntry;
      } catch {
        return null;
      }
    }).filter(Boolean) as LogEntry[];
  } catch {
    return [];
  }
}

/**
 * 获取日志目录中可用的日期列表
 */
function getAvailableDates(logDir: string, level: LogLevel): string[] {
  const dir = resolve(logDir, level);
  if (!existsSync(dir)) return [];

  try {
    return readdirSync(dir)
      .filter((f) => f.endsWith(".jsonl"))
      .map((f) => f.replace(".jsonl", ""))
      .sort()
      .reverse(); // 最新的在前
  } catch {
    return [];
  }
}

/**
 * 按 traceId 查询所有相关日志条目
 * 跨所有日志级别搜索
 */
export function queryByTraceId(
  traceId: string,
  logDir: string = DEFAULT_LOG_DIR
): LogEntry[] {
  const levels: LogLevel[] = ["boundary", "business", "detail"];
  const results: LogEntry[] = [];

  // 获取最近 3 天的日期（trace 日志通常在短时间内产生）
  const dates: string[] = [];
  for (let i = 0; i < 3; i++) {
    const d = new Date();
    d.setDate(d.getDate() - i);
    dates.push(d.toISOString().slice(0, 10));
  }

  for (const level of levels) {
    for (const date of dates) {
      const entries = readJsonlFile(logDir, level, date);
      for (const entry of entries) {
        if (entry.trace === traceId) {
          results.push({ ...entry, _level: level } as LogEntry & { _level: string });
        }
      }
    }
  }

  // 按时间排序
  results.sort((a, b) => a.ts.localeCompare(b.ts));
  return results;
}

/**
 * 按 spanId 查询所有相关日志条目
 */
export function queryBySpanId(
  spanId: string,
  logDir: string = DEFAULT_LOG_DIR
): LogEntry[] {
  const levels: LogLevel[] = ["boundary", "business", "detail"];
  const results: LogEntry[] = [];

  const dates: string[] = [];
  for (let i = 0; i < 3; i++) {
    const d = new Date();
    d.setDate(d.getDate() - i);
    dates.push(d.toISOString().slice(0, 10));
  }

  for (const level of levels) {
    for (const date of dates) {
      const entries = readJsonlFile(logDir, level, date);
      for (const entry of entries) {
        if (entry.span === spanId) {
          results.push(entry);
        }
      }
    }
  }

  results.sort((a, b) => a.ts.localeCompare(b.ts));
  return results;
}

/**
 * 查询最近的日志条目
 */
export function queryRecent(
  options: LogQueryOptions = {},
  logDir: string = DEFAULT_LOG_DIR
): LogEntry[] {
  const { level, op, limit = 100, dateFrom, dateTo } = options;
  const levels: LogLevel[] = level
    ? (Array.isArray(level) ? level : [level])
    : ["boundary", "business", "detail"];
  const ops = op ? (Array.isArray(op) ? op : [op]) : undefined;

  const results: LogEntry[] = [];
  const today = new Date().toISOString().slice(0, 10);

  for (const lv of levels) {
    const dates = getAvailableDates(logDir, lv);
    for (const date of dates) {
      // 日期范围过滤
      if (dateFrom && date < dateFrom) continue;
      if (dateTo && date > dateTo) continue;

      const entries = readJsonlFile(logDir, lv, date);
      for (const entry of entries) {
        if (ops && !ops.includes(entry.op)) continue;
        results.push(entry);
      }

      // 如果已经够了，停止读取更多日期文件
      if (results.length >= limit * 2) break;
    }
  }

  // 按时间倒序排列，取最近的 limit 条
  results.sort((a, b) => b.ts.localeCompare(a.ts));
  return results.slice(0, limit);
}
