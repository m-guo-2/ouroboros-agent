## 朋友圈

通过 wecom_api 工具调用以下方法。

### 浏览朋友圈
- method: `/sns/getSnsRecord`
- params: `{maxId: 0}` （分页，首次传 0）

### 获取动态详情
- method: `/sns/getSnsDetail`
- params: `{snsIds: ["动态ID1"]}`

### 发布朋友圈（需先上传媒体）
1. 上传媒体: method: `/sns/upload`, params: `{fileUrl: "图片URL"}`
2. 发布: method: `/sns/postSns`, params: `{content: "文字内容", mediaList: [上传返回的媒体信息]}`

### 删除朋友圈
- method: `/sns/deleteSns`
- params: `{snsId: "动态ID"}`

### 点赞
- method: `/sns/snsLike`
- params: `{snsId: "动态ID"}`

### 评论
- method: `/sns/snsComment`
- params: `{snsId: "动态ID", content: "评论内容"}`

### 删除评论
- method: `/sns/deleteSnsComment`
- params: `{snsId: "动态ID", commentId: "评论ID"}`