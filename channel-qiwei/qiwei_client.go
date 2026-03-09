package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type qiweiClient struct {
	baseURL    string
	token      string
	guid       string
	httpClient *http.Client
}

type qiweiRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func newQiweiClient(cfg Config) *qiweiClient {
	return &qiweiClient{
		baseURL: cfg.APIBaseURL,
		token:   cfg.Token,
		guid:    cfg.GUID,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RequestTimout) * time.Second,
		},
	}
}

func (c *qiweiClient) doAPIRaw(ctx context.Context, method string, params map[string]any) (qiweiDoAPIResponse, error) {
	reqBody := qiweiRequest{
		Method: normalizeMethod(method),
		Params: mergeParams(c.guid, params),
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return qiweiDoAPIResponse{}, err
	}

	url := c.baseURL + "/api/qw/doApi"
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return qiweiDoAPIResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-QIWEI-TOKEN", c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 2 {
				sleepRetry(ctx, attempt)
				continue
			}
			return qiweiDoAPIResponse{}, err
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return qiweiDoAPIResponse{}, readErr
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("qiwei server error: %d body=%s", resp.StatusCode, string(body))
			if attempt < 2 {
				sleepRetry(ctx, attempt)
				continue
			}
			return qiweiDoAPIResponse{}, lastErr
		}
		if resp.StatusCode >= 400 {
			return qiweiDoAPIResponse{}, fmt.Errorf("qiwei api error: %d body=%s", resp.StatusCode, string(body))
		}

		var out qiweiDoAPIResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return qiweiDoAPIResponse{}, err
		}
		if out.Code != 0 && out.Code != 200 {
			return out, fmt.Errorf("qiwei business error: code=%d msg=%s", out.Code, out.Msg)
		}
		return out, nil
	}

	if lastErr == nil {
		lastErr = errors.New("unknown qiwei request error")
	}
	return qiweiDoAPIResponse{}, lastErr
}

func mergeParams(guid string, params map[string]any) map[string]any {
	out := map[string]any{"guid": guid}
	for k, v := range params {
		out[k] = v
	}
	return out
}

func sleepRetry(ctx context.Context, attempt int) {
	delay := time.Duration(200*(1<<attempt)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func normalizeMethod(method string) string {
	m := strings.TrimSpace(method)
	if m == "" {
		return m
	}
	if strings.HasPrefix(m, "/") {
		return m
	}
	return "/" + m
}
