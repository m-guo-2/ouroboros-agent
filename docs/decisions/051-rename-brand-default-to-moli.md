# 默认品牌名切换为 Moli

- **日期**：2026-03-06
- **类型**：讨论结论
- **状态**：已实施

## 背景

仓库内原先使用 `Ouroboros` 作为默认品牌名与部署示例名，但该名称已不再适合作为当前项目的统一对外标识。与此同时，仓库中已经存在 `moli-system-prompt.md` 等命名线索，切换到 `Moli` 更一致。

## 决策

将仓库中的默认品牌名、管理后台展示名和生产部署默认前缀统一切换为 `Moli`。

## 变更内容

- 将根文档与管理后台文案从 `Ouroboros` / `衔尾蛇` 统一改为 `Moli`。
- 将部署脚本默认 `APP_NAME` 从 `ouroboros` 改为 `moli`。
- 将部署模板文件名统一改为：
  - `deploy/systemd/moli-agent.service`
  - `deploy/systemd/moli-qiwei.service`
  - `deploy/systemd/moli-feishu.service`
  - `deploy/nginx/moli.conf`
- 将部署说明中的路径示例更新为 `/etc/moli`、`/opt/moli`、`moli-agent.service`。

## 考虑过的替代方案

- 仅保留部署层的可配置 `APP_NAME`，不修改仓库默认文案：
  这样虽然能用，但仓库内会长期保留旧品牌名，容易造成文档、页面和部署示例不一致。

## 影响

后续新增文档、页面文案和部署资产应默认使用 `Moli`。若未来真的需要更换公开仓库名或远端仓库地址，应在此基础上单独执行仓库层面的重命名操作。
