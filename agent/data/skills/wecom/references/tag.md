## 标签管理

通过 wecom_api 工具调用以下方法。

### 同步标签列表
- method: `/label/syncLabelList`
- params: `{}`

### 编辑个人标签
- method: `/label/editLabel`
- params: `{labelId: "标签ID", labelName: "标签名", memberIds: ["联系人ID"]}`

### 编辑客户标签
- method: `/label/contactEditLabel`
- params: `{userId: "客户ID", labelIds: ["标签ID"]}`