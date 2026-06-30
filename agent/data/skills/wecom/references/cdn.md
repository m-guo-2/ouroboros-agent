## 文件传输

通过 wecom_api 工具调用以下方法。

### 通过 URL 上传文件
- method: `/cloud/cdnBigUploadByUrl`
- params: `{fileUrl: "文件URL"}`

### 异步上传（大文件）
- method: `/cloud/cdnUploadByUrlAsync`
- params: `{fileUrl: "文件URL"}`

### 下载企业聊天文件
- method: `/cloud/wxWorkDownload`
- params: `{fileId: "文件ID"}`

### 异步下载企业聊天文件
- method: `/cloud/wxWorkDownloadAsync`
- params: `{fileId: "文件ID"}`

### 下载个人聊天文件
- method: `/cloud/wxDownload`
- params: `{fileId: "文件ID"}`

### CDN 文件转 URL
- method: `/cloud/cdnWxDownload`
- params: `{cdnKey: "CDN密钥"}`