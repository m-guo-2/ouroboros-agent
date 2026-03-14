## 1. Tavily client and builtin tool

- [x] 1.1 新增 Tavily client/adapter，封装认证、请求、超时和响应解析
- [x] 1.2 在 agent runtime 中注册 `tavily_search` builtin tool，并定义稳定输入/输出结构
- [x] 1.3 为 `tavily_search` 增加缺少 API Key、被禁用、上游失败等错误处理
- [x] 1.4 为 `tavily_search` 增加结果条数上限、snippet 长度限制，以及 `total_results` / `returned_results` / `truncated` 字段
- [x] 1.5 为 Tavily tool 接入轻量 trace/impact 元数据记录

## 2. Tavily configuration management

- [x] 2.1 在 settings 存储层增加 Tavily 配置 key 的读取约定和默认值处理
- [x] 2.2 在 API 层暴露 Tavily 配置的读取与更新能力，保持与现有 settings 接口一致
- [x] 2.3 在 admin 设置页增加 Tavily API Key、Base URL、启用开关和默认参数字段

## 3. Web research subagent profile

- [x] 3.1 在 `subagent.Manager` 中新增 `web_research` profile 的归一化、展示名和白名单配置
- [x] 3.2 为 `web_research` profile 编写专用 system prompt，强调 Tavily-first 和 parent-facing summary
- [x] 3.3 让 `web_research` profile 仅暴露 Tavily 与必要低风险上下文工具，不开放 Shell 和文件写入
- [x] 3.4 在 `web_research` prompt 中加入“结果过多时先缩窄检索再总结”的约束
- [x] 3.5 在 Tavily 不可用时为 `web_research` 返回明确阻塞原因

## 4. Validation and tests

- [x] 4.1 为 Tavily client 和 `tavily_search` tool 编写成功、配置缺失、上游失败和结果截断测试
- [x] 4.2 为 `web_research` profile 编写 profile 校验、工具白名单和 prompt 约束测试
- [x] 4.3 验证 admin 设置页能正确保存并回显 Tavily 配置
- [x] 4.4 补充实现文档或 decision 记录，说明 Tavily tool 与 `web_research` subagent 的职责边界
