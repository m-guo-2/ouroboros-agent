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

# 保留的配置表
KEEP_TABLES="settings agent_configs models skills"

echo "🔒 将保留的配置表:"
for t in $KEEP_TABLES; do
  count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM $t;" 2>/dev/null || echo "0")
  printf "   %-20s %s 条\n" "$t:" "$count"
done
echo ""

# 要删除的表：除配置表外的所有表
DROP_TABLES=$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' AND name NOT IN ('settings','agent_configs','models','skills','sqlite_sequence') ORDER BY name;")

if [ -z "$DROP_TABLES" ]; then
  echo "没有需要删除的表。"
  exit 0
fi

echo "🗑  将删除的表（DROP TABLE，重启后自动重建）:"
for t in $DROP_TABLES; do
  count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM $t;" 2>/dev/null || echo "?")
  printf "   %-25s %s 条\n" "$t:" "$count"
done
echo ""

read -p "⚠️  确认删除以上表？(y/N) " confirm
if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
  echo "已取消"
  exit 0
fi

# 备份
BACKUP="${DB_PATH}.bak.$(date +%Y%m%d%H%M%S)"
cp "$DB_PATH" "$BACKUP"
echo "💾 已备份到: $BACKUP"

# DROP 所有非配置表
SQL=""
for t in $DROP_TABLES; do
  SQL="${SQL}DROP TABLE IF EXISTS ${t};"
done
SQL="${SQL}DELETE FROM sqlite_sequence WHERE name NOT IN ('settings','agent_configs','models','skills');"
SQL="${SQL}VACUUM;"

sqlite3 "$DB_PATH" "$SQL"

echo "✅ 数据表已删除，配置已保留"
echo "   重启应用后将以新 schema 自动重建。"
echo ""
echo "🔒 保留的配置:"
for t in $KEEP_TABLES; do
  count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM $t;" 2>/dev/null || echo "0")
  printf "   %-20s %s 条\n" "$t:" "$count"
done
