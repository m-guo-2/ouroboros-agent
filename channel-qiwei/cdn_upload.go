package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

// cdnUploadResult holds the response from /cloud/cdnBigUploadByUrl.
// All media messages (image, file, voice, video) require uploading to CDN first,
// then sending with the CDN-returned parameters. See https://doc.qiweapi.com/api-344613900.md
type cdnUploadResult struct {
	FileAesKey    string `json:"fileAesKey"`
	FileID        string `json:"fileId"`
	FileKey       string `json:"fileKey"`
	FileMd5       string `json:"fileMd5"`
	FileSize      int    `json:"fileSize"`
	FileThumbSize int    `json:"fileThumbSize"`
	CloudURL      string `json:"cloudUrl"`
	Filename      string `json:"filename"`
}

func (a *app) cdnUploadByURL(ctx context.Context, rt *accountRuntime, fileURL, filename string, fileType int) (*cdnUploadResult, error) {
	logger.Business(ctx, "CDN 上传开始",
		"accountId", rt.AccountID(),
		"fileUrl", fileURL,
		"filename", filename,
		"fileType", fileType,
	)

	res, err := rt.client.doAPIRaw(ctx, "/cloud/cdnBigUploadByUrl", map[string]any{
		"fileUrl":  fileURL,
		"filename": filename,
		"fileType": fileType,
	})
	if err != nil {
		return nil, fmt.Errorf("cdn upload: %w", err)
	}

	var result cdnUploadResult
	if err := json.Unmarshal(res.Data, &result); err != nil {
		return nil, fmt.Errorf("cdn upload response decode: %w", err)
	}

	logger.Business(ctx, "CDN 上传完成",
		"fileId", result.FileID,
		"fileSize", result.FileSize,
		"filename", result.Filename,
	)
	return &result, nil
}

func (a *app) cdnUploadFile(ctx context.Context, rt *accountRuntime, filePath, filename string, fileType int) (*cdnUploadResult, error) {
	logger.Business(ctx, "CDN 文件直传开始",
		"accountId", rt.AccountID(),
		"filePath", filePath,
		"filename", filename,
		"fileType", fileType,
	)

	res, err := rt.client.doFileAPIRaw(ctx, "/cloud/cdnBigUpload", filePath, fileType, filename)
	if err != nil {
		return nil, fmt.Errorf("cdn file upload: %w", err)
	}

	var result cdnUploadResult
	if err := json.Unmarshal(res.Data, &result); err != nil {
		return nil, fmt.Errorf("cdn file upload response decode: %w", err)
	}
	if strings.TrimSpace(result.Filename) == "" {
		result.Filename = filename
	}

	logger.Business(ctx, "CDN 文件直传完成",
		"fileId", result.FileID,
		"fileSize", result.FileSize,
		"filename", result.Filename,
	)
	return &result, nil
}

// resolveMediaSendParams uploads content to CDN and returns the bridge method + params
// for sending the media message. This is the correct two-step flow:
//  1. Upload local files to CDN via /cloud/cdnBigUpload, or public URLs via /cloud/cdnBigUploadByUrl
//  2. Send via /msg/send* with CDN-returned file params
func (a *app) resolveMediaSendParams(ctx context.Context, rt *accountRuntime, messageType, toID, content string, meta map[string]any) (string, map[string]any, error) {
	filename := mediaFilename(messageType, content, meta)
	fileType := mediaFileType(messageType)

	var cdn *cdnUploadResult
	var err error
	if localPath, ok := localUploadPath(content); ok {
		if filename == "" || filename == "file.dat" || filename == filepath.Base(content) {
			filename = filepath.Base(localPath)
		}
		cdn, err = a.cdnUploadFile(ctx, rt, localPath, filename, fileType)
	} else {
		cdn, err = a.cdnUploadDownloadedURL(ctx, rt, content, filename, fileType)
		if err != nil {
			logger.Warn(ctx, "CDN 下载直传失败，降级 URL 上传",
				"accountId", rt.AccountID(),
				"content", content,
				"error", err.Error(),
			)
			cdn, err = a.cdnUploadByURL(ctx, rt, content, filename, fileType)
		}
	}
	if err != nil {
		return "", nil, err
	}

	if strings.TrimSpace(cdn.Filename) == "" {
		cdn.Filename = filename
	}

	method, params := buildMediaSendParams(messageType, toID, cdn, meta)
	return method, params, nil
}

func (a *app) cdnUploadDownloadedURL(ctx context.Context, rt *accountRuntime, fileURL, filename string, fileType int) (*cdnUploadResult, error) {
	if !strings.HasPrefix(fileURL, "http://") && !strings.HasPrefix(fileURL, "https://") {
		return nil, fmt.Errorf("not an HTTP URL")
	}
	resp, err := a.http.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download file: HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "qiwei-cdn-upload-*"+filepath.Ext(filename))
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	return a.cdnUploadFile(ctx, rt, tmpPath, filename, fileType)
}

func localUploadPath(content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" || strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") || strings.HasPrefix(content, "oss://") {
		return "", false
	}
	if !filepath.IsAbs(content) && !strings.Contains(content, string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(content)
	if err != nil || info.IsDir() {
		return "", false
	}
	return content, true
}

func isMediaMessageType(messageType string) bool {
	switch messageType {
	case "image", "gif", "file", "voice", "video":
		return true
	default:
		return false
	}
}

// mediaFileType maps message types to qiweapi CDN upload file types.
// 1 = jpg/image, 4 = mp4/video, 5 = generic file (including amr voice)
func mediaFileType(messageType string) int {
	switch messageType {
	case "image", "gif":
		return 1
	case "video":
		return 4
	default:
		return 5
	}
}

func mediaFilename(messageType, contentURL string, meta map[string]any) string {
	if meta != nil {
		if v, ok := meta["fileName"].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	if u, err := url.Parse(contentURL); err == nil {
		base := path.Base(u.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	switch messageType {
	case "image":
		return "image.jpg"
	case "gif":
		return "image.gif"
	case "video":
		return "video.mp4"
	case "voice":
		return "voice.amr"
	default:
		return "file.dat"
	}
}

// buildMediaSendParams constructs the method and params for /msg/send* APIs
// using the CDN upload result. Each message type has slightly different required fields.
// Docs: https://doc.qiweapi.com
func buildMediaSendParams(messageType, toID string, cdn *cdnUploadResult, meta map[string]any) (string, map[string]any) {
	params := map[string]any{
		"toId":       toID,
		"fileAesKey": cdn.FileAesKey,
		"fileId":     cdn.FileID,
		"fileSize":   cdn.FileSize,
	}

	switch messageType {
	case "image":
		params["fileKey"] = cdn.FileKey
		params["fileMd5"] = cdn.FileMd5
		params["filename"] = cdn.Filename
		return "/msg/sendImage", params

	case "gif":
		params["imgUrl"] = cdn.CloudURL
		return "/msg/sendGif", params

	case "file":
		params["filename"] = cdn.Filename
		return "/msg/sendFile", params

	case "voice":
		voiceTime := 0
		if meta != nil {
			if v := anyToInt64(meta["voiceTime"]); v > 0 {
				voiceTime = int(v)
			}
		}
		params["voiceTime"] = voiceTime
		return "/msg/sendVoice", params

	case "video":
		params["fileMd5"] = cdn.FileMd5
		params["filename"] = cdn.Filename
		params["coverImageSize"] = cdn.FileThumbSize
		duration := 0
		if meta != nil {
			if v := anyToInt64(meta["duration"]); v > 0 {
				duration = int(v)
			}
		}
		params["duration"] = duration
		return "/msg/sendVideo", params

	default:
		params["filename"] = cdn.Filename
		return "/msg/sendFile", params
	}
}
