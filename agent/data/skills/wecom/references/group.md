## 群管理

通过 wecom_api 工具调用以下方法。所有 params 中无需传 guid（系统自动注入）。

### 创建群
- method: `/room/createRoom`
- params: `{memberIds: ["id1","id2"], roomName: "群名"}`

### 修改群名
- method: `/room/modifyRoomName`
- params: `{roomId: "群ID", name: "新群名"}`

### 修改群公告
- method: `/room/modifyRoomNotice`
- params: `{roomId: "群ID", notice: "公告内容"}`

### 添加群成员
- method: `/room/inviteRoomMember`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 移除群成员
- method: `/room/removeRoomMember`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 设置群管理员
- method: `/room/roomAddAdmin`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 取消群管理员
- method: `/room/roomRemoveAdmin`
- params: `{roomId: "群ID", memberIds: ["id1"]}`

### 转让群主
- method: `/room/changeRoomMaster`
- params: `{roomId: "群ID", memberId: "新群主ID"}`

### 解散群
- method: `/room/dismissRoom`
- params: `{roomId: "群ID"}`

### 退出群
- method: `/room/quitRoom`
- params: `{roomId: "群ID"}`

### 获取群二维码
- method: `/room/getRoomQrCode`
- params: `{roomId: "群ID"}`

### 设置群内昵称
- method: `/room/modifyRoomNickname`
- params: `{roomId: "群ID", nickname: "昵称"}`