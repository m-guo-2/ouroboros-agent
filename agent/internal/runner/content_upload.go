package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
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

var mediaDownloadClient = &http.Client{Timeout: 30 * time.Second}

// resolveMediaContent ensures content is an OSS-backed presigned URL
// for media message types (file, image, voice). All media goes through OSS
// so that downstream channel adapters can reliably access the content.
//
//   - OSS presigned URL → pass through
//   - http(s):// (non-OSS) → download → upload to OSS → presigned URL
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

	if isHTTPURL(content) && isOwnOSSURL(content) {
		return content, nil
	}

	storage := cardrender.OSSStorage()
	if storage == nil {
		if err := cardrender.Init(); err == nil {
			storage = cardrender.OSSStorage()
		}
	}
	if storage == nil {
		return "", fmt.Errorf("媒体发送需要 OSS 存储配置，当前不可用")
	}

	if isHTTPURL(content) {
		return reuploadHTTPMedia(ctx, storage, content)
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

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".bmp":  true,
}

// looksLikeFilePath returns true when content looks like a filesystem path
// rather than a natural-language message. Used to gate an os.Stat check so
// we don't stat every outgoing text message.
func looksLikeFilePath(content string) bool {
	if content == "" {
		return false
	}
	if strings.Contains(content, "://") {
		return false
	}
	if strings.Contains(content, "\n") {
		return false
	}
	if strings.Count(content, " ") > 1 {
		return false
	}
	if strings.HasPrefix(content, "/") {
		return true
	}
	return strings.Contains(content, "/")
}

// inferMessageTypeFromFile stats the path and returns "image" or "file"
// based on its extension. Returns "" if the file does not exist or is a
// directory.
func inferMessageTypeFromFile(filePath string) string {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if imageExtensions[ext] {
		return "image"
	}
	return "file"
}

func isOwnOSSURL(rawURL string) bool {
	return isOSSURLForEndpoint(cardrender.OSSEndpoint(), rawURL)
}

func isOSSURLForEndpoint(endpoint, rawURL string) bool {
	if endpoint == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.Host == endpoint
}

func reuploadHTTPMedia(ctx context.Context, storage oss.Storage, sourceURL string) (string, error) {
	resp, err := mediaDownloadClient.Get(sourceURL)
	if err != nil {
		return "", fmt.Errorf("下载媒体文件失败 %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载媒体文件失败 %s: HTTP %d", sourceURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取媒体文件失败 %s: %w", sourceURL, err)
	}

	parsed, _ := url.Parse(sourceURL)
	fileName := filepath.Base(parsed.Path)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "media"
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName)))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	key, err := oss.GenerateObjectKey(uploadKeyPrefix, fileName)
	if err != nil {
		return "", fmt.Errorf("生成 OSS key 失败: %w", err)
	}

	_, err = storage.PutObject(ctx, oss.PutObjectInput{
		Key:         key,
		FileName:    fileName,
		ContentType: contentType,
		Size:        int64(len(body)),
		Body:        bytes.NewReader(body),
	})
	if err != nil {
		return "", fmt.Errorf("媒体文件上传 OSS 失败: %w", err)
	}

	presignedURL, err := storage.PresignGetURL(ctx, key, uploadURLExpiry)
	if err != nil {
		return "", fmt.Errorf("生成预签名 URL 失败: %w", err)
	}

	logger.Business(ctx, "HTTP 媒体已中转上传 OSS",
		"sourceURL", sourceURL,
		"ossKey", key,
		"fileName", fileName,
		"size", len(body),
	)

	return presignedURL, nil
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
