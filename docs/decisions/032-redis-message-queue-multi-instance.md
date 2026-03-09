# Redis 消息队列 + HRW 多实例调度

- **日期**：2026-03-04
- **类型**：架构决策
- **状态**：待实施

## 背景

当前 Agent 使用内存队列（`map[sessionID]*SessionWorker`）处理来自飞书/企微的消息。存在三个根本问题：

1. **进程重启消息丢失**：内存队列无持久化，重启或崩溃时未处理的消息直接消失。
2. **单实例瓶颈**：所有 session 的 LLM 调用串行/并发在单进程内，无法水平扩展。
3. **Channel 强耦合 Agent**：Channel 通过 HTTP POST 直连 Agent，Agent 宕机时 Channel 侧报错。

需要一套基于 Redis 的轻量级消息队列，支持多实例部署、确定性路由和分布式互斥。

## 决策

### 总体架构

```
┌──────────┐  ┌──────────┐
│  Feishu  │  │  QiWei   │
└────┬─────┘  └────┬─────┘
     │              │
     │  compute sessionKey = channel:conversationId
     │  LPUSH queue:session:{sessionKey}
     │  PUBLISH notify:new_message
     │              │
     ▼              ▼
┌──────────────────────────────────────────────────────┐
│                       Redis                           │
│                                                       │
│  queue:session:{sk}   — List, 每 session 一个队列      │
│  agent:instances      — Sorted Set, 实例注册表         │
│  lock:session:{sk}    — String + NX, 分布式锁          │
│  notify:new_message   — Pub/Sub, 唤醒通知              │
│  notify:instance_chg  — Pub/Sub, 实例变更通知          │
│  sessions:active      — Set, 有消息待处理的 sessionKey │
└──────────────────────────────────────────────────────┘
        │               │               │
        ▼               ▼               ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ Agent Inst-A │ │ Agent Inst-B │ │ Agent Inst-C │
│              │ │              │ │              │
│ HRW 判定归属  │ │ HRW 判定归属  │ │ HRW 判定归属  │
│ 获取分布式锁  │ │ 获取分布式锁  │ │ 获取分布式锁  │
│ Dispatcher   │ │ Dispatcher   │ │ Dispatcher   │
│ Runner       │ │ Runner       │ │ Runner       │
└──────────────┘ └──────────────┘ └──────────────┘
```

三层各司其职：**Queue** 解耦和缓冲、**HRW** 确定性路由避免争抢、**Lock** 保证过渡期正确性。

### Channel 端：推入 Redis 队列

Channel 不再 HTTP POST 到 Agent，而是直接写 Redis：

```go
func (a *app) forwardToAgent(ctx context.Context, in incomingMessage) error {
    sessionKey := resolveSessionKey(in.Channel, in.ChannelConversationID, in.ChannelUserID)
    payload, _ := json.Marshal(in)

    pipe := a.redis.Pipeline()
    pipe.LPush(ctx, "queue:session:"+sessionKey, payload)
    pipe.SAdd(ctx, "sessions:active", sessionKey)
    pipe.Publish(ctx, "notify:new_message", sessionKey)
    _, err := pipe.Exec(ctx)
    return err
}
```

WebUI / API 走同样的路径——Agent 实例接收 HTTP 后自己 LPUSH 到 Redis（保留 `/api/channels/incoming` 作为入口）。

### 实例注册（Instance Registry）

```
Redis Sorted Set: agent:instances
  member = instanceId（环境变量 AGENT_INSTANCE_ID, 如 "agent-01"）
  score  = 最后心跳时间戳（Unix ms）
```

- 每个实例启动时 `ZADD`，每 **5s** 心跳更新 score。
- 存活判定：`score > now - 15s`。
- 获取存活实例：`ZRANGEBYSCORE agent:instances (now-15000) +inf`。
- 优雅关停时 `ZREM` 并 `PUBLISH notify:instance_chg`。

### HRW 哈希路由

```go
func hrwHash(sessionKey, instanceID string) uint64 {
    h := fnv.New64a()
    h.Write([]byte(sessionKey))
    h.Write([]byte{0})
    h.Write([]byte(instanceID))
    return h.Sum64()
}

func selectOwner(sessionKey string, instances []string) string {
    var maxHash uint64
    var owner string
    for _, inst := range instances {
        if h := hrwHash(sessionKey, inst); h > maxHash {
            maxHash = h
            owner = inst
        }
    }
    return owner
}
```

HRW（Rendezvous Hashing）优势：
- 无虚拟节点、无哈希环，代码 < 20 行。
- 增删实例只影响 `1/N` 的 session 重分配。
- 确定性：所有实例对同一 sessionKey 计算出相同 owner。

### 分布式锁（Session Lock）

```
Redis Key:  lock:session:{sessionKey}
Value:      instanceId
TTL:        120s
获取:       SET lock:session:{sk} {instanceId} NX EX 120
续约:       每 30s 续约（TTL 的 1/4，留足容错窗口）
释放:       Lua 脚本原子 DEL（仅当值 == 自己的 instanceId）
```

释放锁 Lua 脚本：

```lua
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
```

续约 Lua 脚本：

```lua
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
```

锁的角色：HRW 负责效率（避免 N 个实例争抢），锁负责正确性（过渡期的互斥保证）。

### 锁续约与丢锁中断

续约 goroutine 在连续失败 3 次后主动 cancel 处理上下文，中断当前处理，避免双写。

```go
func renewLockPeriodically(ctx context.Context, sk string, interval time.Duration, cancel context.CancelFunc) {
    failCount := 0
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            ok := renewLock(sk)
            if !ok {
                failCount++
                if failCount >= 3 {
                    cancel()
                    return
                }
            } else {
                failCount = 0
            }
        }
    }
}
```

闭环：**锁丢失 → 续约失败 → cancel context → processOneEvent 感知 ctx.Done() 中断 → consumeSession 退出 → 释放资源**。新 owner 拿到锁后从队列继续消费。

### 消费循环（Consumer Loop）

每个 Agent 实例启动后运行消费循环：

```
1. Subscribe notify:new_message
2. Subscribe notify:instance_chg

On new_message(sessionKey):
  instances ← ZRANGEBYSCORE agent:instances ...
  owner ← HRW(sessionKey, instances)
  if owner != me → skip
  if already_processing(sessionKey) → skip
  go consumeSession(sessionKey)

On instance_chg:
  重新评估 sessions:active 中的归属
  接管新归属的 session

Periodic reconciliation (every 30s):
  for sk in SMEMBERS sessions:active:
    if HRW owner == me && not processing:
      go consumeSession(sk)
```

`consumeSession` 实现：

```go
func consumeSession(sessionKey string) {
    // 1. 获取分布式锁
    if !acquireLock(sessionKey) {
        return
    }
    defer releaseLock(sessionKey)

    // 2. 启动锁续约
    processCtx, cancelProcess := context.WithCancel(ctx)
    defer cancelProcess()
    go renewLockPeriodically(ctx, sessionKey, 30*time.Second, cancelProcess)

    // 3. 排空队列
    for {
        select {
        case <-processCtx.Done():
            return // 锁丢失，中断
        default:
        }

        raw, err := redis.RPop("queue:session:" + sessionKey)
        if err == redis.Nil {
            redis.SRem("sessions:active", sessionKey)
            return
        }

        var msg IncomingMessage
        json.Unmarshal(raw, &msg)

        // 4. 完整 Dispatcher 逻辑（去重、用户、Session、存消息）
        result := Dispatch(processCtx, msg)
        if result.Duplicate {
            continue
        }

        // 5. 直接处理（不再入内存队列）
        processOneEvent(processCtx, result.SessionID, ...)
    }
}
```

### 数据流对比

| 步骤 | 现状 | 改造后 |
|------|------|--------|
| Channel 投递 | HTTP POST → Agent | LPUSH → Redis |
| 去重 | Dispatcher 同步检查 | Consumer 侧检查 |
| 队列 | 内存 `map[sessionID][]Request` | Redis List per session |
| 路由 | 无（单实例） | HRW 哈希 |
| 互斥 | 内存 mutex + single goroutine | Redis 分布式锁 + 续约 |
| 处理 | 本地 goroutine | 锁持有者处理 |

## 边界场景

### 滚动更新 / 重启

```
t0: Inst-A 处理 Session-X
t1: Inst-A 收到 SIGTERM
    → ZREM agent:instances（注销）
    → PUBLISH notify:instance_chg
    → 停止接受新 session
    → 等待当前 Session-X 处理完成（grace period 30s）
    → 释放 Session-X 的锁
t2: Inst-B 收到 instance_chg
    → 重新计算 HRW，发现 Session-X 归自己
    → 尝试获取锁（等 A 释放或锁过期）
    → 开始消费 Session-X 的队列
t3: Inst-A'（新版本）启动
    → ZADD agent:instances
    → PUBLISH notify:instance_chg
    → 部分 session 可能重新路由到 A'
```

保证：消息不丢（在 Redis 中），不重复处理（去重 + 锁），中断可恢复（消息已落库，session context 在 DB）。

### 实例崩溃（非优雅退出）

- 心跳停止 → 15s 后其他实例判定其死亡。
- 周期 reconciliation 发现 orphaned sessions。
- 新 owner 等待锁过期（最多 120s）后接管。
- **优化**：reconciliation 时发现锁持有者不在存活列表中，用 Lua 强制释放：

```lua
local holder = redis.call("GET", KEYS[1])
if holder and not redis.call("ZSCORE", "agent:instances", holder) then
    return redis.call("DEL", KEYS[1])
end
return 0
```

### Redis 不可用

- Channel 侧：LPUSH 失败 → fallback 到 HTTP POST `/api/channels/incoming`（保留旧路径作为降级）。
- Agent 侧：消费循环断开 → 重连 with exponential backoff。
- 不做本地队列缓冲（增加复杂度，收益小）。

### 消息堆积

- 监控 `LLEN queue:session:{sk}` 和 `SCARD sessions:active`。
- 某 session 队列过长（>100）时告警或限流。

## Redis Key 总览

| Key | Type | 用途 | TTL |
|-----|------|------|-----|
| `queue:session:{sk}` | List | 每 session 的消息队列 | 无（排空后 DEL） |
| `sessions:active` | Set | 有待处理消息的 session 集合 | 无 |
| `agent:instances` | Sorted Set | 实例注册，score=心跳时间 | 无（ZREM 或心跳过期） |
| `lock:session:{sk}` | String | 分布式锁，value=instanceId | 120s |
| `notify:new_message` | Pub/Sub | 新消息通知 | — |
| `notify:instance_chg` | Pub/Sub | 实例变更通知 | — |

## 代码改动范围

| 模块 | 变更 |
|------|------|
| `agent/internal/redis/` | **新增** Redis 客户端封装（`go-redis/v9`） |
| `agent/internal/cluster/` | **新增** 实例注册、HRW、分布式锁、消费循环 |
| `agent/internal/runner/worker.go` | **重构** 移除内存 `sessionWorkers`，替换为 Redis 消费 |
| `agent/internal/dispatcher/dispatcher.go` | **重构** `HandleIncoming` 改为 LPUSH Redis（WebUI/API 入口） |
| `agent/cmd/agent/main.go` | **修改** 初始化 Redis、启动 cluster、优雅关停 |
| `channel-feishu/` | **修改** `forwardToAgent` → Redis LPUSH |
| `channel-qiwei/` | **修改** `forwardToAgent` → Redis LPUSH |

## 前置依赖

| 依赖 | 说明 | 优先级 |
|------|------|--------|
| Redis 实例 | 开发环境 Docker，生产环境托管 Redis | P0 |
| 共享存储 | 多实例需共享 sessions/messages 数据；当前 SQLite 单进程锁，需迁移到 PostgreSQL | P0，可并行推进 |
| `go-redis/v9` | Go Redis 客户端 | 随本方案引入 |

## 分阶段实施

| 阶段 | 内容 | 复杂度 |
|------|------|--------|
| Phase 0 | 引入 Redis 客户端，实现实例注册 + 心跳 + HRW 库 | 低 |
| Phase 1 | Agent 侧：Dispatcher LPUSH Redis，Consumer 消费循环替换内存队列，分布式锁 | 中 |
| Phase 2 | Channel 侧：feishu/qiwei 改为直接 LPUSH Redis，Agent 保留 HTTP 入口作为 fallback | 中 |
| Phase 3 | 存储层迁移到 PostgreSQL（多实例共享数据） | 中高 |
| Phase 4 | 监控、告警、运维工具（队列深度、锁状态、实例拓扑可视化） | 低 |

单实例部署时（无 Redis），可保留当前内存队列作为降级路径，通过 `REDIS_URL` 环境变量是否存在来切换模式。
