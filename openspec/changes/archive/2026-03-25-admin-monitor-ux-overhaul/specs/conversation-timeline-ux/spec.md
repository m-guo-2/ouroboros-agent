## ADDED Requirements

### Requirement: 语义化消息角色标签
对话时间线中的用户消息 SHALL 根据来源类型使用不同的语义标签，替代统一的 "外部事件"。

#### Scenario: 普通用户消息
- **WHEN** 消息的 `initiator` 为 "user" 或为空
- **THEN** 标签显示为 "用户消息"，使用 brand 色调

#### Scenario: 系统触发消息
- **WHEN** 消息的 `initiator` 为 "system" 或 exchange 的 `isSystemInitiated` 为 true
- **THEN** 标签显示为 "系统触发"，使用灰色调

#### Scenario: 定时任务触发
- **WHEN** 消息的 `initiator` 为 "scheduled" 或 "delayed_task"
- **THEN** 标签显示为 "定时任务"，使用琥珀色调

#### Scenario: 其他 initiator 类型
- **WHEN** 消息的 `initiator` 为其他值
- **THEN** 标签显示 initiator 原始值

### Requirement: 消息复制按钮
对话时间线中每条消息 SHALL 提供 hover 时显示的复制按钮。

#### Scenario: hover 用户消息或助手消息
- **WHEN** 用户将鼠标悬停在一条消息上
- **THEN** 消息右上角出现复制图标按钮

#### Scenario: 点击复制按钮
- **WHEN** 用户点击复制按钮
- **THEN** 消息的纯文本内容被复制到剪贴板
- **THEN** 按钮变为 "已复制" 状态，1.5 秒后恢复

### Requirement: 滚动到底部按钮
当用户向上滚动查看历史消息后，对话时间线 SHALL 显示 "滚动到底部" 浮动按钮。

#### Scenario: 用户不在底部
- **WHEN** 对话时间线的滚动位置距离底部超过 200px
- **THEN** 右下角出现 "↓" 浮动按钮

#### Scenario: 点击滚动到底部
- **WHEN** 用户点击浮动按钮
- **THEN** 对话时间线平滑滚动到底部
- **THEN** 浮动按钮消失

#### Scenario: 用户已在底部
- **WHEN** 对话时间线已滚动到底部（距底部 < 60px）
- **THEN** 不显示浮动按钮

### Requirement: timeAgo 自动刷新
所有 timeAgo 显示的相对时间 SHALL 自动定期更新。

#### Scenario: 时间经过后更新
- **WHEN** 一条消息显示 "1 分钟前"，又过了 2 分钟
- **THEN** 显示自动更新为 "3 分钟前"，无需用户操作

#### Scenario: 刷新间隔
- **WHEN** 页面处于前台
- **THEN** 每 60 秒触发一次所有 timeAgo 组件的重新计算
