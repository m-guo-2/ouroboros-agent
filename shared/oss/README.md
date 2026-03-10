# Shared OSS Module

`shared/oss` 是一个独立的 Go module，提供 MinIO/S3 兼容对象存储的公共上传/下载能力。

## 配置

通过环境变量读取运行时配置：

```bash
export OSS_ENDPOINT=115.190.14.209:2012
export OSS_BUCKET=your-bucket
export OSS_ACCESS_KEY=your-access-key
export OSS_SECRET_KEY=your-secret-key
export OSS_USE_SSL=false
export OSS_REGION=us-east-1
export OSS_PREFIX=agent/uploads
export OSS_TIMEOUT_SECONDS=30
```

如果 `OSS_ENDPOINT` 写成 `http://115.190.14.209:2012` 或 `https://...`，模块会自动剥离 scheme，并同步修正 `UseSSL`。

## 使用示例

```go
package example

import (
	"context"
	"strings"

	oss "github.com/m-guo-2/ouroboros-agent/shared/oss"
)

func uploadText() error {
	cfg, err := oss.LoadConfigFromEnv().Normalized()
	if err != nil {
		return err
	}

	store, err := oss.NewMinIOStorage(cfg)
	if err != nil {
		return err
	}

	result, err := store.PutObject(context.Background(), oss.PutObjectInput{
		FileName:    "hello.txt",
		ContentType: "text/plain; charset=utf-8",
		Size:        int64(len("hello world")),
		Body:        strings.NewReader("hello world"),
	})
	if err != nil {
		return err
	}

	downloaded, err := store.GetObject(context.Background(), result.Key)
	if err != nil {
		return err
	}
	defer downloaded.Body.Close()

	return nil
}
```

## 错误分类

调用方可以使用 `errors.Is` 判断稳定错误类型：

```go
if errors.Is(err, oss.ErrNotFound) { /* ... */ }
if errors.Is(err, oss.ErrAuthentication) { /* ... */ }
if errors.Is(err, oss.ErrTransport) { /* ... */ }
```

## 测试替身

单元测试里可以注入 `oss.NewFakeStorage()`，避免依赖真实 MinIO 服务。
