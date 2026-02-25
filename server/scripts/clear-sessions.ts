#!/usr/bin/env bun
/**
 * 清理数据库中所有session相关的数据
 * 用于从头开始测试
 */

import { Database } from "bun:sqlite";
import { resolve } from "path";

const DB_PATH = resolve(import.meta.dir, "../data/config.db");

console.log(`🗑️  开始清理session数据...`);
console.log(`📁 数据库路径: ${DB_PATH}`);

const db = new Database(DB_PATH);

// 开始事务
db.run("BEGIN TRANSACTION");

try {
  // 1. 删除所有消息（关联到session）
  const messagesResult = db.run("DELETE FROM messages");
  console.log(`✅ 删除了 ${messagesResult.changes} 条消息记录`);

  // 2. 删除所有session记录
  const sessionsResult = db.run("DELETE FROM agent_sessions");
  console.log(`✅ 删除了 ${sessionsResult.changes} 条session记录`);

  // 3. 删除关联到session的用户记忆事实
  const memoryFactsResult = db.run("DELETE FROM user_memory_facts WHERE source_session_id IS NOT NULL");
  console.log(`✅ 删除了 ${memoryFactsResult.changes} 条关联到session的用户记忆事实`);

  // 4. 删除关联到session的Agent笔记
  const notesResult = db.run("DELETE FROM agent_notes WHERE related_session_id IS NOT NULL");
  console.log(`✅ 删除了 ${notesResult.changes} 条关联到session的Agent笔记`);

  // 5. 删除关联到session的Agent任务
  const tasksResult = db.run("DELETE FROM agent_tasks WHERE source_session_id IS NOT NULL");
  console.log(`✅ 删除了 ${tasksResult.changes} 条关联到session的Agent任务`);

  // 提交事务
  db.run("COMMIT");
  console.log(`\n✨ 所有session相关数据已清理完成！`);
} catch (error) {
  // 回滚事务
  db.run("ROLLBACK");
  console.error(`❌ 清理失败:`, error);
  process.exit(1);
} finally {
  db.close();
}
