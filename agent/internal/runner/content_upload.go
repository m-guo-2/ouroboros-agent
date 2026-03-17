package runner

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent/internal/cardrender"
	"agent/internal/logger"

	"github.com/m-guo-2/ouroboros-agent/shared/oss"
)

const (
	uploadKeyPrefix = "channel-uploads"
	uploadURLExpiry = 7 * 24 * time.Hour
)

// resolveMediaContent ensures content is a publicly accessible HTTP(S) URL
// for media message types (file, image, voice).
//   - http(s):// → pass through
//   - oss://bucket/key → presigned URL
//   - local file path → upload to OSS → presigned URL
func resolveMediaContent(ctx context.Context, messageType, content string) (string, error) {
	if !isMediaMessageType(messageType) {
		return content, nil
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return content, nil
	}
	if isHTTPURL(content) {
		return content, nil
	}

	storage := cardrender.OSSStorage()
	if storage == nil {
		if err := cardrender.Init(); err == nil {
			storage = cardrender.OSSStorage()
		}
	}
	if storage == nil {
		return "", fmt.Errorf("文件发送需要 OSS 存储配置，当前不可用。content 必须是 http(s):// 开头的可公开访问 URL")
	}

	if strings.HasPrefix(content, "oss://") {
		return resolveOSSURI(ctx, storage, content)
	}

	return uploadLocalFile(ctx, storage, content)
}

func isMediaMessageType(messageType string) bool {
	switch messageType {
	case "file", "image", "voice":
		return true
	default:
		return false
	}
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func uploadLocalFile(ctx context.Context, storage oss.Storage, localPath string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("无法读取文件 %s: %w", localPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("无法获取文件信息 %s: %w", localPath, err)
	}

	fileName := filepath.Base(localPath)
	key, err := oss.GenerateObjectKey(uploadKeyPrefix, fileName)
	if err != nil {
		return "", fmt.Errorf("生成 OSS key 失败: %w", err)
	}

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = storage.PutObject(ctx, oss.PutObjectInput{
		Key:         key,
		FileName:    fileName,
		ContentType: contentType,
		Size:        stat.Size(),
		Body:        f,
	})
	if err != nil {
		return "", fmt.Errorf("文件上传 OSS 失败: %w", err)
	}

	url, err := storage.PresignGetURL(ctx, key, uploadURLExpiry)
	if err != nil {
		return "", fmt.Errorf("生成预签名 URL 失败: %w", err)
	}

	logger.Business(ctx, "本地文件已上传 OSS",
		"localPath", localPath,
		"ossKey", key,
		"fileName", fileName,
		"size", stat.Size(),
	)

	return url, nil
}

func resolveOSSURI(ctx context.Context, storage oss.Storage, ossURI string) (string, error) {
	trimmed := strings.TrimPrefix(ossURI, "oss://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("无效的 OSS URI: %s", ossURI)
	}
	key := strings.TrimSpace(parts[1])

	url, err := storage.PresignGetURL(ctx, key, uploadURLExpiry)
	if err != nil {
		return "", fmt.Errorf("OSS URI 转换为预签名 URL 失败: %w", err)
	}

	logger.Business(ctx, "OSS URI 已转换为预签名 URL",
		"ossURI", ossURI,
		"ossKey", key,
	)

	return url, nil
}
