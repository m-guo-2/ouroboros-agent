---
name: skill-design-guide
description: "Design and build service-oriented skills that wrap external APIs. Provides architecture patterns for splitting intent-handling (markdown) from API orchestration (scripts). Use when creating a new skill that integrates with an external service, or when refactoring an existing API-heavy skill."
---

# 服务型 Skill 设计指南

当一个 skill 需要对接外部 API（如腾讯文档、地图服务、企业微信等），本指南提供架构决策框架。

## 核心判断：Markdown 还是代码？

```
这个操作需要 agent 发挥创造力吗？
  ├── 是 → Markdown（高自由度）
  │        例：构思文档内容、分析用户需求、决定数据结构
  └── 否 → Python 脚本（低自由度）
           例：多步 API 编排、参数格式转换、异步轮询、错误重试
```

**底线原则**：如果一个操作是固定的多步 API 编排（步骤确定、顺序固定、参数可预知），它就不该用自然语言教 agent 做，而该用代码固化。

## 两层架构

```
skill-name/
├── SKILL.md              # Skill 层：能力菜单 + 调用方式 + 约束
├── scripts/              # 脚本服务层：封装 API 编排
│   ├── client.py         #   底层 HTTP/SDK 封装
│   ├── service_a.py      #   业务服务 A（CLI 入口）
│   └── service_b.py      #   业务服务 B（CLI 入口）
└── references/           # 冷备参考：仅当脚本未覆盖的高级操作需要时
    └── api_reference.md
```

| 层 | 载体 | 职责 | Agent 读取时机 |
|---|------|------|-------------|
| Skill 层 | SKILL.md | 能力索引、使用方法、关键约束 | 每次都读 |
| 脚本服务层 | scripts/*.py | 执行多步 API 编排 | 不读，只执行 |
| 参考层 | references/*.md | 底层 API 完整参数 | 仅高级/异常场景按需读 |

引用严格一层深：SKILL.md → scripts/ 或 SKILL.md → references/。禁止 SKILL.md → 中间文件 → references/。

## SKILL.md 写法

### 只写 agent 不知道的

Agent 已经很聪明。不要教它"先理解需求再动手" — 它本来就会。只提供它没有的领域知识：

- API 怎么调（工具名、参数结构）
- 脚本怎么用（命令 + 参数）
- 有什么坑（格式约束、异步轮询规则、级联删除风险）
- 文档类型怎么选（领域特定的决策规则）

### 结构模板

```markdown
---
name: xxx
description: "WHAT it does. WHEN to use it."
---

# 标题

## 调用方式
工具/脚本的调用格式。

## 能力速查
| 需要做什么 | 命令 |
|-----------|------|
| 场景 A    | `python scripts/xxx.py action --param value` |
| 场景 B    | `python scripts/xxx.py action --param value` |

## 关键约束
agent 不可能自己知道的规则和限制。

## 参考文档
高级操作时按需查阅的 reference 文件列表。
```

### 不该出现的内容

| 写了没用的 | 为什么 |
|-----------|-------|
| "先理解用户需求" | Agent 本来就会 |
| "用 Markdown 组织内容" | Agent 本来就会 |
| 每个意图举 3 个路由示例 | Agent 本来就能做意图分类 |
| 多步 API 编排的自然语言伪代码 | 应该固化在脚本里 |

## 脚本服务层设计

### client.py — 底层封装

统一处理：认证（Token）、HTTP 调用、错误处理、响应解析。所有业务脚本通过 client 调用 API，不直接发 HTTP 请求。

```python
class Client:
    def call(self, name: str, arguments: dict) -> dict:
        """调用一个 API 操作，返回结果 dict"""

    def poll(self, submit_name, progress_name, submit_args,
             interval=5, timeout=300, done_check=None) -> dict:
        """提交异步任务 + 自动轮询直到完成"""
```

### 业务脚本 — CLI 入口

每个脚本对外暴露 CLI 命令，输出 JSON。Agent 执行命令，解析输出。

```bash
python scripts/service.py <action> [--param value ...]
```

设计原则：

1. **一个命令完成一件事** — agent 调一次就够，不需要连续调多次
2. **内部封装编排** — 多步 API 调用、中间状态传递、默认值清理都在脚本内部
3. **JSON 输出** — 统一用 JSON 返回结果，方便 agent 解析
4. **错误信息有用** — 失败时输出可操作的错误信息，不是原始 HTTP 错误

### 异步任务的统一模式

外部 API 常有异步操作（创建 PPT、导入文件、网页剪藏等），统一封装在 client.poll()：

```
提交任务 → 自动轮询 → 返回最终结果
```

Agent 不需要知道轮询逻辑，只看到一次调用和最终结果。

## 从已有 API skill 迁移

如果已有一个 API-heavy 的 skill（大量 references、工作流伪代码），按以下步骤迁移：

```
1. 盘点所有 API，按业务场景分组
2. 识别高频的多步编排模式 → 这些变成脚本的 action
3. 写 client.py 封装底层调用
4. 逐个实现业务脚本
5. 重写 SKILL.md 为能力菜单（指向脚本命令）
6. references/ 保留为冷备
7. 删除中间层文件（如 services/*.md）
```

## 验证清单

- [ ] SKILL.md < 500 行
- [ ] SKILL.md 没有教 agent 它已经知道的事
- [ ] 所有多步 API 编排都在脚本中，不在 markdown 中
- [ ] 引用层级 = 1（SKILL.md 直接指向 scripts/ 或 references/）
- [ ] 脚本输出 JSON，错误信息可操作
- [ ] 每个脚本命令一次完成一件完整的事
- [ ] description 包含 WHAT + WHEN，第三人称
