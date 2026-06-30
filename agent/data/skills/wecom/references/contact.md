## 联系人管理

通过 wecom_api 工具调用以下方法。

### 获取联系人详情（批量）
- method: `/contact/batchGetUserinfo`
- params: `{userIds: ["id1","id2"]}`

### 列出个人联系人
- method: `/contact/getWxContactList`
- params: `{}`

### 列出企业联系人
- method: `/contact/getWxWorkContactList`
- params: `{}`

### 添加个人好友
- method: `/contact/addSearchWxContact`
- params: `{keyword: "手机号或微信号", verifyContent: "验证消息"}`

### 添加企业好友
- method: `/contact/addSearchWxWorkContact`
- params: `{keyword: "搜索词"}`

### 通过好友申请
- method: `/contact/agreeContact`
- params: `{encryptUserName: "加密用户名", ticket: "ticket"}`

### 修改个人联系人备注
- method: `/contact/updateWxContact`
- params: `{userId: "联系人ID", remark: "新备注"}`

### 修改企业联系人备注
- method: `/contact/updateWxWorkContact`
- params: `{userId: "联系人ID", remark: "新备注"}`

### 删除联系人
- method: `/contact/deleteContact`
- params: `{userId: "联系人ID"}`