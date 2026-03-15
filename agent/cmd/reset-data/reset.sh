#!/usr/bin/env bash
set -euo pipefail

DB_PATH="${1:-/opt/moli/data/config.db}"

if [ ! -f "$DB_PATH" ]; then
  echo "❌ 数据库不存在: $DB_PATH"
  echo "用法: $0 [数据库路径]"
  echo "示例: $0 data/config.db"
  exit 1
fi

echo "📦 数据库: $DB_PATH"
echo ""

# 展示将保留的配置数据
echo "🔒 将保留的配置表:"
echo "   agent_configs: $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM agent_configs;") 条"
echo "   models:        $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM models;") 条"
echo "   skills:        $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM skills;") 条"
echo "   settings:      $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM settings;") 条"
echo ""

echo "🗑  将清空的数据表:"
echo "   messages:            $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM messages;") 条"
echo "   agent_sessions:      $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM agent_sessions;") 条"
echo "   users:               $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM users;") 条"
echo "   user_channels:       $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM user_channels;") 条"
echo "   user_memory:         $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM user_memory;") 条"
echo "   user_memory_facts:   $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM user_memory_facts;") 条"
echo "   session_facts:       $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM session_facts;") 条"
echo "   context_compactions: $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM context_compactions;") 条"
echo "   delayed_tasks:       $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM delayed_tasks;") 条"
echo "   processed_messages:  $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM processed_messages;") 条"
echo ""

read -p "⚠️  确认清空以上数据表？(y/N) " confirm
if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
  echo "已取消"
  exit 0
fi

# 备份
BACKUP="${DB_PATH}.bak.$(date +%Y%m%d%H%M%S)"
cp "$DB_PATH" "$BACKUP"
echo "💾 已备份到: $BACKUP"

# 清空数据表，保留配置表
sqlite3 "$DB_PATH" <<'SQL'
DELETE FROM messages;
DELETE FROM agent_sessions;
DELETE FROM users;
DELETE FROM user_channels;
DELETE FROM user_memory;
DELETE FROM user_memory_facts;
DELETE FROM session_facts;
DELETE FROM context_compactions;
DELETE FROM delayed_tasks;
DELETE FROM processed_messages;
VACUUM;
SQL

echo "✅ 数据已清空，配置已保留"
echo ""
echo "🔒 保留的配置:"
echo "   agent_configs: $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM agent_configs;") 条"
echo "   models:        $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM models;") 条"
echo "   skills:        $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM skills;") 条"
echo "   settings:      $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM settings;") 条"
