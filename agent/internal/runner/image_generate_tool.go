package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"agent/internal/cardrender"
	"agent/internal/storage"
	"agent/internal/types"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
	"github.com/m-guo-2/ouroboros-agent/shared/oss"
)

const (
	defaultImageProvider  = "volcengine"
	defaultImageModel     = "doubao-seedream-5-0-260128"
	defaultImageSize      = "2K"
	imageGenerateExpiry   = 7 * 24 * time.Hour
	imageGeneratePrefix   = "generated/images"
	maxReferenceImages    = 14
	maxReferenceImageSize = 30 * 1024 * 1024
)

func registerImageGenerateTool(registry interface {
	RegisterBuiltin(string, string, types.JSONSchema, types.ToolExecutor)
}, sessionID string) {
	registry.RegisterBuiltin("image_generate",
		"根据专业生图提示词生成或编辑图片。用户要求参考当前会话中的图片时，必须通过 referenceAttachmentIds 传入对应附件 ID；外部参考图才使用 referenceImages URL。严格数字报表优先使用 data_report/render_card。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"prompt":                 map[string]interface{}{"type": "string", "description": "生图或编辑提示词。建议先通过 visual-prompt-designer skill 生成或润色"},
				"size":                   map[string]interface{}{"type": "string", "description": "图片尺寸或清晰度，例如 2K、4K、1024x1024。默认 2K"},
				"model":                  map[string]interface{}{"type": "string", "description": "可选模型名，默认 doubao-seedream-5-0-260128"},
				"provider":               map[string]interface{}{"type": "string", "description": "可选 provider，默认 volcengine"},
				"referenceAttachmentIds": map[string]interface{}{"type": "array", "description": "当前会话中的参考图片附件 ID 列表。编辑用户发来的图片时使用", "items": map[string]interface{}{"type": "string"}, "maxItems": maxReferenceImages},
				"referenceImages":        map[string]interface{}{"type": "array", "description": "外部参考图 URL 列表；当前会话附件不要使用此参数", "items": map[string]interface{}{"type": "string"}, "maxItems": maxReferenceImages},
				"watermark":              map[string]interface{}{"type": "boolean", "description": "是否添加水印；不传则交给模型服务默认值"},
				"seed":                   map[string]interface{}{"type": "integer", "description": "可选随机种子"},
				"extraParameters":        map[string]interface{}{"type": "object", "description": "透传给图片生成 API 的少量额外参数"},
			},
			Required: []string{"prompt"},
		},
		createImageGenerateExecutor(sessionID),
	)
}

func createImageGenerateExecutor(sessionID string) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		prompt, _ := input["prompt"].(string)
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			return nil, fmt.Errorf("prompt is required")
		}

		provider := stringValue(input, "provider", defaultImageProvider)
		model := normalizeImageModel(provider, stringValue(input, "model", defaultImageModel))
		size := normalizeImageSize(provider, stringValue(input, "size", defaultImageSize))

		creds, err := storage.GetProviderCredentials(provider)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(creds.APIKey) == "" {
			return nil, fmt.Errorf("provider %q API key is not configured", provider)
		}

		referenceAttachmentIDs := stringSliceValue(input["referenceAttachmentIds"])
		refs := stringSliceValue(input["referenceImages"])
		attachmentRefs, err := resolveReferenceAttachmentImages(ctx, sessionID, referenceAttachmentIDs)
		if err != nil {
			return nil, err
		}
		refs = append(refs, attachmentRefs...)
		if len(refs) > maxReferenceImages {
			return nil, fmt.Errorf("image_generate supports at most %d reference images", maxReferenceImages)
		}

		body := map[string]interface{}{
			"model":  model,
			"prompt": prompt,
			"size":   size,
		}
		if len(refs) > 0 {
			body["image"] = refs
		}
		if watermark, ok := input["watermark"].(bool); ok {
			body["watermark"] = watermark
		}
		if seed, ok := numberToInt(input["seed"]); ok {
			body["seed"] = seed
		}
		if extra := mapValue(input["extraParameters"]); extra != nil {
			for k, v := range extra {
				k = strings.TrimSpace(k)
				if k != "" {
					body[k] = v
				}
			}
		}

		respBytes, err := callImageGenerationAPI(ctx, creds.BaseURL, creds.APIKey, body)
		if err != nil {
			return nil, err
		}
		image, err := firstGeneratedImage(respBytes)
		if err != nil {
			return nil, err
		}

		imageURL, ossKey, err := persistGeneratedImage(ctx, image)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"imageUrl":               imageURL,
			"ossKey":                 ossKey,
			"model":                  model,
			"provider":               provider,
			"size":                   size,
			"prompt":                 prompt,
			"referenceAttachmentIds": referenceAttachmentIDs,
		}, nil
	}
}

func resolveReferenceAttachmentImages(ctx context.Context, sessionID string, attachmentIDs []string) ([]string, error) {
	if len(attachmentIDs) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("reference attachments require an active session")
	}

	storageClient := cardrender.OSSStorage()
	if storageClient == nil {
		if err := cardrender.Init(); err == nil {
			storageClient = cardrender.OSSStorage()
		}
	}
	if storageClient == nil {
		return nil, fmt.Errorf("reference attachments require OSS storage")
	}

	refs := make([]string, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		attachment, ok := storage.FindAttachmentInSession(sessionID, attachmentID)
		if !ok {
			return nil, fmt.Errorf("reference attachment %q was not found in the current session", attachmentID)
		}
		if attachment.Kind != "image" {
			return nil, fmt.Errorf("reference attachment %q is %q, not an image", attachmentID, attachment.Kind)
		}
		ref, err := referenceAttachmentDataURL(ctx, storageClient, attachment)
		if err != nil {
			return nil, fmt.Errorf("read reference attachment %q: %w", attachmentID, err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func referenceAttachmentDataURL(ctx context.Context, storageClient oss.Storage, attachment storage.AttachmentData) (string, error) {
	resourceURI := strings.TrimSpace(attachment.ResourceURI)
	if !strings.HasPrefix(resourceURI, "oss://") {
		return "", fmt.Errorf("unsupported attachment resource URI %q", resourceURI)
	}
	trimmed := strings.TrimPrefix(resourceURI, "oss://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("invalid OSS resource URI %q", resourceURI)
	}

	object, err := storageClient.GetObject(ctx, strings.TrimSpace(parts[1]))
	if err != nil {
		return "", err
	}
	defer object.Body.Close()
	if object.Size > maxReferenceImageSize {
		return "", fmt.Errorf("image is too large: %d bytes", object.Size)
	}

	raw, err := io.ReadAll(io.LimitReader(object.Body, maxReferenceImageSize+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxReferenceImageSize {
		return "", fmt.Errorf("image is too large: more than %d bytes", maxReferenceImageSize)
	}
	mimeType := strings.TrimSpace(attachment.MIMEType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = strings.TrimSpace(object.ContentType)
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(raw)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "", fmt.Errorf("unsupported image content type %q", mimeType)
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func callImageGenerationAPI(ctx context.Context, baseURL, apiKey string, body map[string]interface{}) ([]byte, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/images/generations"
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := sharedlogger.NewClient("image-generate-tool", 600*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("image generation failed: HTTP %d %s", resp.StatusCode, string(respBytes))
	}
	return respBytes, nil
}

type responsesImageGenerationRequest struct {
	Prompt          string
	Model           string
	ImageModel      string
	Size            string
	ReferenceImages []string
	ExtraParameters map[string]interface{}
}

func callResponsesImageGenerationAPI(ctx context.Context, baseURL, apiKey string, spec responsesImageGenerationRequest) (generatedImage, error) {
	content := []map[string]interface{}{
		{"type": "input_text", "text": spec.Prompt},
	}
	for _, ref := range spec.ReferenceImages {
		content = append(content, map[string]interface{}{"type": "input_image", "image_url": ref})
	}

	tool := map[string]interface{}{
		"type":          "image_generation",
		"model":         spec.ImageModel,
		"size":          spec.Size,
		"output_format": "png",
	}
	for k, v := range spec.ExtraParameters {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		switch k {
		case "quality", "background", "output_format", "moderation":
			tool[k] = v
		}
	}

	body := map[string]interface{}{
		"model":        spec.Model,
		"instructions": "Create the requested image. Return the generated image through the image_generation tool.",
		"input": []map[string]interface{}{
			{
				"type":    "message",
				"role":    "user",
				"content": content,
			},
		},
		"tools":       []map[string]interface{}{tool},
		"tool_choice": map[string]interface{}{"type": "image_generation"},
		"stream":      true,
		"store":       false,
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/responses"
	payload, err := json.Marshal(body)
	if err != nil {
		return generatedImage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return generatedImage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := sharedlogger.NewClient("image-generate-tool", 600*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return generatedImage{}, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return generatedImage{}, fmt.Errorf("image generation failed: HTTP %d %s", resp.StatusCode, string(respBytes))
	}
	image, err := firstResponsesGeneratedImage(respBytes)
	if err != nil {
		return generatedImage{}, err
	}
	return image, nil
}

type generatedImage struct {
	URL       string
	B64JSON   string
	DataURI   string
	MediaType string
}

func firstGeneratedImage(respBytes []byte) (generatedImage, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return generatedImage{}, fmt.Errorf("invalid image generation response: %w", err)
	}

	if images := stringSliceValue(raw["images"]); len(images) > 0 {
		return imageFromString(images[0]), nil
	}
	if data, ok := raw["data"].([]interface{}); ok && len(data) > 0 {
		if item, ok := data[0].(map[string]interface{}); ok {
			if s, _ := item["url"].(string); strings.TrimSpace(s) != "" {
				return generatedImage{URL: strings.TrimSpace(s)}, nil
			}
			if s, _ := item["imageURL"].(string); strings.TrimSpace(s) != "" {
				return generatedImage{URL: strings.TrimSpace(s)}, nil
			}
			if s, _ := item["b64_json"].(string); strings.TrimSpace(s) != "" {
				return generatedImage{B64JSON: strings.TrimSpace(s), MediaType: "image/png"}, nil
			}
			if s, _ := item["imageBase64Data"].(string); strings.TrimSpace(s) != "" {
				return imageFromString(s), nil
			}
			if s, _ := item["imageDataURI"].(string); strings.TrimSpace(s) != "" {
				return imageFromString(s), nil
			}
		}
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if s, _ := data["download_url"].(string); strings.TrimSpace(s) != "" {
			return generatedImage{URL: strings.TrimSpace(s)}, nil
		}
		if s, _ := data["url"].(string); strings.TrimSpace(s) != "" {
			return generatedImage{URL: strings.TrimSpace(s)}, nil
		}
	}
	return generatedImage{}, fmt.Errorf("image generation response did not contain a supported image URL or base64 field")
}

func firstResponsesGeneratedImage(respBytes []byte) (generatedImage, error) {
	trimmed := bytes.TrimSpace(respBytes)
	if len(trimmed) == 0 {
		return generatedImage{}, fmt.Errorf("image generation response was empty")
	}

	if image, ok, err := extractImageFromJSON(trimmed); ok || err != nil {
		return image, err
	}

	events := strings.Split(string(respBytes), "\n\n")
	for _, event := range events {
		lines := strings.Split(event, "\n")
		var data strings.Builder
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if value == "[DONE]" {
					continue
				}
				data.WriteString(value)
			}
		}
		if data.Len() == 0 {
			continue
		}
		image, ok, err := extractImageFromJSON([]byte(data.String()))
		if ok || err != nil {
			return image, err
		}
	}

	return generatedImage{}, fmt.Errorf("image generation response did not contain an image_generation result")
}

func extractImageFromJSON(respBytes []byte) (generatedImage, bool, error) {
	var raw interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return generatedImage{}, false, nil
	}
	if msg, ok := responseErrorMessage(raw); ok {
		return generatedImage{}, true, fmt.Errorf("image generation failed: %s", msg)
	}
	if result, ok := findImageGenerationResult(raw); ok {
		return imageFromString(result), true, nil
	}
	return generatedImage{}, false, nil
}

func responseErrorMessage(value interface{}) (string, bool) {
	obj, ok := value.(map[string]interface{})
	if !ok {
		return "", false
	}
	if typ, _ := obj["type"].(string); typ == "error" {
		if errObj, ok := obj["error"].(map[string]interface{}); ok {
			if msg, _ := errObj["message"].(string); strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg), true
			}
		}
		if msg, _ := obj["message"].(string); strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg), true
		}
	}
	if errObj, ok := obj["error"].(map[string]interface{}); ok {
		if msg, _ := errObj["message"].(string); strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg), true
		}
	}
	return "", false
}

func findImageGenerationResult(value interface{}) (string, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		if typ, _ := v["type"].(string); typ == "image_generation_call" {
			if result, _ := v["result"].(string); strings.TrimSpace(result) != "" {
				return strings.TrimSpace(result), true
			}
		}
		for _, key := range []string{"item", "response", "output", "content", "data"} {
			if result, ok := findImageGenerationResult(v[key]); ok {
				return result, true
			}
		}
		for _, nested := range v {
			if result, ok := findImageGenerationResult(nested); ok {
				return result, true
			}
		}
	case []interface{}:
		for _, item := range v {
			if result, ok := findImageGenerationResult(item); ok {
				return result, true
			}
		}
	}
	return "", false
}

func imageFromString(value string) generatedImage {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return generatedImage{URL: value}
	}
	if strings.HasPrefix(value, "data:") {
		return generatedImage{DataURI: value}
	}
	return generatedImage{B64JSON: value, MediaType: "image/png"}
}

func persistGeneratedImage(ctx context.Context, image generatedImage) (string, string, error) {
	if image.URL != "" {
		return image.URL, "", nil
	}

	mediaType := image.MediaType
	data := image.B64JSON
	if image.DataURI != "" {
		var ok bool
		mediaType, data, ok = splitDataURI(image.DataURI)
		if !ok {
			return "", "", fmt.Errorf("invalid data URI image result")
		}
	}
	if mediaType == "" {
		mediaType = "image/png"
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", "", fmt.Errorf("decode generated image failed: %w", err)
	}

	storageClient := cardrender.OSSStorage()
	if storageClient == nil {
		if err := cardrender.Init(); err == nil {
			storageClient = cardrender.OSSStorage()
		}
	}
	if storageClient == nil {
		return "", "", fmt.Errorf("image_generate requires OSS storage to persist base64 image results")
	}

	ext := extensionForMediaType(mediaType)
	key, err := oss.GenerateObjectKey(imageGeneratePrefix, "image"+ext)
	if err != nil {
		return "", "", err
	}
	putResult, err := storageClient.PutObject(ctx, oss.PutObjectInput{
		Key:         key,
		FileName:    path.Base(key),
		ContentType: mediaType,
		Size:        int64(len(decoded)),
		Body:        bytes.NewReader(decoded),
	})
	if err != nil {
		return "", "", err
	}
	url, err := storageClient.PresignGetURL(ctx, putResult.Key, imageGenerateExpiry)
	if err != nil {
		return "", "", err
	}
	return url, putResult.Key, nil
}

func splitDataURI(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "data:") {
		return "", "", false
	}
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	header := strings.TrimPrefix(parts[0], "data:")
	if !strings.HasSuffix(header, ";base64") {
		return "", "", false
	}
	return strings.TrimSuffix(header, ";base64"), parts[1], true
}

func extensionForMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func stringValue(input map[string]interface{}, key, fallback string) string {
	if s, _ := input[key].(string); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return fallback
}

func normalizeImageSize(provider, size string) string {
	size = strings.TrimSpace(size)
	if isVolcengineProvider(provider) && size == "1024x1024" {
		return defaultImageSize
	}
	switch strings.ToLower(size) {
	case "":
		return defaultImageSize
	case "2k":
		return "2K"
	case "3k":
		return "3K"
	case "4k":
		return "4K"
	default:
		return size
	}
}

func normalizeImageModel(provider, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return defaultImageModel
	}
	if isVolcengineProvider(provider) {
		if !strings.HasPrefix(model, "doubao-seedream-") {
			return defaultImageModel
		}
	}
	return model
}

func isVolcengineProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "volcengine", "ark":
		return true
	default:
		return false
	}
}

func mapValue(value interface{}) map[string]interface{} {
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return nil
}

func stringSliceValue(value interface{}) []string {
	raw, ok := value.([]interface{})
	if !ok {
		if typed, ok := value.([]string); ok {
			return typed
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, _ := item.(string); strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func numberToInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
