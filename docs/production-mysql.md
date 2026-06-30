# 生产 MySQL 连接信息

本文记录当前生产环境使用的 MySQL/RDS 连接信息。只记录非密钥信息；密码不要提交到仓库。

## 连接信息

- Host：`rm-bp143yp44432a3bu4ao.mysql.rds.aliyuncs.com`
- Port：`3306`
- User：`admin421`
- Agent 数据库：`moli_agent`
- Qiwei 数据库：`moli_qiwei`

## 服务器配置位置

生产服务器：`moli-prod`

- Agent 配置：`/opt/moli/conf/agent.yaml`
- Qiwei 配置：`/opt/moli/conf/qiwei.yaml`

这两个配置文件中包含完整 MySQL 配置和密码，权限应保持为仅服务器侧可读，不要复制到仓库。

## 当前状态

截至 2026-05-21，业务数据已从 SQLite 切换到 MySQL：

- `moli-agent` 使用 `moli_agent`
- `moli-qiwei` 使用 `moli_qiwei`
- `/opt/moli/data/config.db` 和 `/opt/moli/data/qiwei.db` 是迁移前的旧业务 SQLite 文件，不再作为运行时业务库使用。
- `/opt/moli/logs/sqlite/*.db` 仍可能存在，这是日志/观测落盘，不是业务数据库。

## 使用建议

本地或服务器执行迁移、检查脚本时，优先通过环境变量传递 DSN 或密码，避免把密码出现在 shell 历史、进程参数或 Git 提交中。

```bash
MYSQL_HOST=rm-bp143yp44432a3bu4ao.mysql.rds.aliyuncs.com
MYSQL_PORT=3306
MYSQL_USER=admin421
MYSQL_DATABASE=moli_agent
```

密码从服务器配置文件、密钥管理或临时安全通道读取。
