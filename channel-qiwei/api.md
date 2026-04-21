# 接入流程

:::highlight purple 
**步骤1：** 进入[首页](http://manager.qiweapi.com/index)，点击去开通按钮，领取7天免费试用

<Background bgColor="">
    ![步骤1.png](https://api.apifox.com/api/v1/projects/7051713/resources/585407/image-preview)
</Background>
:::
:::highlight purple 
**步骤2：** 兑换成功后，点击[Token中心](http://manager.qiweapi.com/token)，生成Token，复制TokenId
<Background>
![步骤2.png](https://api.apifox.com/api/v1/projects/7051713/resources/585411/image-preview) 
</Background>
:::

:::highlight purple 
**步骤3：** 打开[API文档](https://doc.qiweapi.com/api-344613850)，携带TokenId参数即可进行登录模块代码接入，登录成功后，即可根据业务调用对应接口开发各类业务。
<Background>
![image.png](https://api.apifox.com/api/v1/projects/7051713/resources/585412/image-preview)
     </Background>
:::

<Card title="1. 如何发送消息‌" > 
在与微信交互中，开发者需先调用[获取联系人模块](https://doc.qiweapi.com/api-344613869)或者[获取群模块](https://doc.qiweapi.com/api-344613881)，获取好友/群的列表缓存入库，然后直接调用[发送消息相关接口](https://doc.qiweapi.com/api-344613906)即可。
</Card>

<Card title="2. 如何接收消息" > 
在与微信交互中，用户可能会处理 好友/群 的消息接收，做到消息交互，可以[配置消息订阅](https://doc.qiweapi.com/doc-7331303)，然后编写业务逻辑，在调用发送消息相关接口即可。
</Card>
<Card title="3.如何开发群管理、自动化等操作" > 
市面上所有机器人操作，都是基于接收消息后的逻辑处理，例如群管理、消息保存、聚合聊天、消息托管、多群转发、内容直播、社区团购、消息转播、云发单、机器人自动回复等，所以开发者只需要[配置消息订阅](https://doc.qiweapi.com/doc-7331303)，再加上业务逻辑即可自定义自己的机器人/客服系统
</Card>
<Card title="4.如何最快测试" > 
平台提供了[在线登录](http://manager.qiweapi.com/loginRecord)，在网页界面就能运行登录，登陆成功后，复制guid及token参数，在线apifox请求测试即可，新用户未编写代码前可使用如上方式测试业务可行性。
</Card>

## Channel ↔ Agent 契约（多账号）

`channel-qiwei` 对接 Agent 时遵循"账号对 Agent 透明"的契约：Agent 不需要知道底下具体是哪一个企微号，只需要把 `channelConversationId` 原样带回即可正确路由到原账号。

### `channelConversationId`

- 形态：`{rawId}@{shortHash}`
  - `rawId`：`FromRoomID` 为群消息时为群 ID，否则为 `SenderID`。
  - `shortHash`：账号 `guid` 的 SHA-256 前 5 字节再做 base32（无填充、小写）后形成的 8 字符后缀。
- 单账号部署时向下兼容：不带 `@` 后缀的 `channelConversationId` 会落到默认（唯一）账号。
- 多账号部署时不带后缀会报 `ErrAmbiguousAccount`，下游需要补 `account_id` 或补 `@shortHash`。

### `channelIdentity`（incoming）

每个 `/api/channels/incoming` 事件会额外携带一个展示用的身份快照，Agent 可直接渲染：

```json
{
  "channelIdentity": {
    "displayName": "主号运营",
    "self": {
      "userId": "wxid_xxx",
      "name": "张三",
      "alias": "@ops",
      "corpName": "ACME"
    }
  }
}
```

`channelIdentity` 仅供展示；**不得用于路由**。路由依据仍然是 `channelConversationId` / `account_id`。

### 下行接口 `account_id` 支持

`/api/qiwei/send`、`/api/qiwei/do`、`/api/qiwei/{module}/{action}`、facade 一族（`search_targets / list_or_get_conversations / parse_message / send_message / get_group_detail / get_contact_detail`）都接受可选 `account_id`。优先级：

1. 请求体 `account_id` 显式指定。
2. `channelConversationId` 末尾的 `@shortHash` 反查。
3. 以上都没有，且进程里仅一个账号，走默认账号；多账号则报错。
