package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalidConfig = errors.New("missing required FEISHU_APP_ID or FEISHU_APP_SECRET")

type feishuClient struct {
	appID     string
	appSecret string
	baseURL   string
	http      *http.Client

	mu          sync.Mutex
	token       string
	tokenExpire time.Time
}

func newFeishuClient(cfg Config) *feishuClient {
	return &feishuClient{
		appID:     cfg.AppID,
		appSecret: cfg.AppSecret,
		baseURL:   "https://open.feishu.cn",
		http: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *feishuClient) doJSON(ctx context.Context, method, path string, query url.Values, body any) (map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(raw)
	}

	reqURL := c.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	token, err := c.getTenantToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out map[string]any
	if len(respRaw) == 0 {
		out = map[string]any{"code": 0, "msg": "ok"}
	} else if err := json.Unmarshal(respRaw, &out); err != nil {
		return nil, fmt.Errorf("invalid feishu response: %s", string(respRaw))
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("feishu http %d: %s", resp.StatusCode, string(respRaw))
	}
	if code := intValue(out["code"]); code != 0 {
		return nil, fmt.Errorf("feishu api error code=%d msg=%v", code, out["msg"])
	}
	return out, nil
}

func (c *feishuClient) uploadImage(ctx context.Context, imageType string, data []byte, filename string) (string, error) {
	if imageType == "" {
		imageType = "message"
	}
	res, err := c.uploadMultipart(ctx, "/open-apis/im/v1/images", map[string]string{
		"image_type": imageType,
	}, "image", filename, data)
	if err != nil {
		return "", err
	}

	dataObj, _ := res["data"].(map[string]any)
	key, _ := dataObj["image_key"].(string)
	if key == "" {
		return "", errors.New("missing image_key in upload response")
	}
	return key, nil
}

func (c *feishuClient) uploadFile(ctx context.Context, fileType, fileName string, data []byte, duration string) (string, error) {
	fields := map[string]string{
		"file_type": fileType,
		"file_name": fileName,
	}
	if duration != "" {
		fields["duration"] = duration
	}
	res, err := c.uploadMultipart(ctx, "/open-apis/im/v1/files", fields, "file", fileName, data)
	if err != nil {
		return "", err
	}

	dataObj, _ := res["data"].(map[string]any)
	key, _ := dataObj["file_key"].(string)
	if key == "" {
		return "", errors.New("missing file_key in upload response")
	}
	return key, nil
}

func (c *feishuClient) uploadMultipart(
	ctx context.Context,
	path string,
	fields map[string]string,
	fileField string,
	fileName string,
	data []byte,
) (map[string]any, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile(fileField, filepath.Base(fileName))
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	token, err := c.getTenantToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("invalid multipart response: %s", string(raw))
	}
	if resp.StatusCode >= 400 || intValue(out["code"]) != 0 {
		return nil, fmt.Errorf("feishu upload failed status=%d code=%d msg=%v", resp.StatusCode, intValue(out["code"]), out["msg"])
	}
	return out, nil
}

func (c *feishuClient) getTenantToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExpire) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	body, _ := json.Marshal(map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Code != 0 || out.TenantAccessToken == "" {
		return "", fmt.Errorf("get token failed code=%d msg=%s", out.Code, out.Msg)
	}

	exp := out.Expire
	if exp <= 0 {
		exp = 7200
	}
	c.mu.Lock()
	c.token = out.TenantAccessToken
	c.tokenExpire = time.Now().Add(time.Duration(exp-60) * time.Second)
	c.mu.Unlock()
	return out.TenantAccessToken, nil
}

func intValue(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	default:
		return 0
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mapStringAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func normalizeLongText(text string) (string, string) {
	if len(text) < 800 {
		return `{"text":` + quote(text) + `}`, "text"
	}

	lines := strings.Split(text, "\n")
	rows := make([][]map[string]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, []map[string]string{{"tag": "text", "text": line}})
	}
	raw, _ := json.Marshal(map[string]any{
		"zh_cn": map[string]any{
			"title":   "",
			"content": rows,
		},
	})
	return string(raw), "post"
}

func quote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}
