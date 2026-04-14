package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
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

func (a *app) cdnUploadByURL(ctx context.Context, fileURL, filename string, fileType int) (*cdnUploadResult, error) {
	logger.Business(ctx, "CDN 上传开始",
		"fileUrl", fileURL,
		"filename", filename,
		"fileType", fileType,
	)

	res, err := a.client.doAPIRaw(ctx, "/cloud/cdnBigUploadByUrl", map[string]any{
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

// resolveMediaSendParams uploads content to CDN and returns the bridge method + params
// for sending the media message. This is the correct two-step flow:
//   1. Upload to CDN via /cloud/cdnBigUploadByUrl
//   2. Send via /msg/send* with CDN-returned file params
func (a *app) resolveMediaSendParams(ctx context.Context, messageType, toID, contentURL string, meta map[string]any) (string, map[string]any, error) {
	filename := mediaFilename(messageType, contentURL, meta)
	fileType := mediaFileType(messageType)

	cdn, err := a.cdnUploadByURL(ctx, contentURL, filename, fileType)
	if err != nil {
		return "", nil, err
	}

	if strings.TrimSpace(cdn.Filename) == "" {
		cdn.Filename = filename
	}

	method, params := buildMediaSendParams(messageType, toID, cdn, meta)
	return method, params, nil
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
