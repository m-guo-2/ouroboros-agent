# GitHub 仓库名与本地目录规划为 moli

- **日期**：2026-03-06
- **类型**：讨论结论
- **状态**：已决定

## 背景

仓库默认品牌名已切换为 `Moli`，但当前 GitHub 仓库远端仍为 `m-guo-2/ouroboros-agent.git`，本地工作目录仍为 `cc_code`。如果不继续统一，品牌、远端地址与本地路径会长期不一致。

## 决策

将 GitHub 仓库名规划为 `moli`，并将本地工作目录规划为 `~/code/moli`。仓库内部的模块目录与 Go module 名暂不随本轮一起改动。

## 变更内容

- 目标远端仓库名：`m-guo-2/moli`
- 目标本地目录名：`/Users/guomiao/code/moli`
- 当前仓库代码结构保持不变：
  - `agent/`
  - `channel-feishu/`
  - `channel-qiwei/`
  - `admin/`
- 本轮优先统一品牌与部署默认名，不把 Go module、进程内部 import 路径和子目录名耦合进一次性大重命名。

## 考虑过的替代方案

- 继续保留 `ouroboros-agent` 作为远端仓库名：
  会造成品牌名和仓库名持续分裂，后续文档、部署说明和对外传播都要反复解释。
- 同时重命名仓库、根目录、模块目录和 Go module：
  变更面过大，容易把品牌切换和工程结构改造混在一起，增加回归风险。

## 影响

后续实际执行时，需要同步处理：

- GitHub 仓库 rename
- 本地目录 rename
- `git remote set-url origin`
- CI/CD、Webhook、部署脚本中引用旧仓库名的配置

建议将“仓库名/目录名改动”与“模块/包路径改动”拆成两次独立操作。
