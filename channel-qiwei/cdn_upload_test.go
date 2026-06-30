package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveMediaSendParamsDownloadsAndDirectUploadsFile(t *testing.T) {
	var receivedMethod, receivedGUID, receivedFileType, receivedFilename, receivedToken string
	var receivedBody string

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/qw/doFileApi" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		receivedToken = r.Header.Get("X-QIWEI-TOKEN")
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		receivedMethod = r.FormValue("method")
		receivedGUID = r.FormValue("guid")
		receivedFileType = r.FormValue("fileType")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		receivedFilename = header.Filename
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		receivedBody = string(body)

		_ = json.NewEncoder(w).Encode(qiweiDoAPIResponse{
			Code: 0,
			Msg:  "成功",
			Data: mustRawJSON(t, map[string]any{
				"fileAesKey": "aes-key",
				"fileId":     "file-id",
				"fileKey":    "file-key",
				"fileMd5":    "md5",
				"fileSize":   12,
			}),
		})
	}))
	defer api.Close()

	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		_, _ = w.Write([]byte("docx-content"))
	}))
	defer download.Close()

	app := &app{http: download.Client()}
	rt := newAccountRuntime(Config{APIBaseURL: api.URL, RequestTimout: 5}, nil, Account{
		ID:      "qw_test",
		GUID:    "guid-1",
		Token:   "token-1",
		Enabled: true,
	})

	method, params, err := app.resolveMediaSendParams(
		context.Background(),
		rt,
		"file",
		"user-1",
		download.URL+"/office-proof.docx",
		map[string]any{"fileName": "office-proof.docx"},
	)
	if err != nil {
		t.Fatalf("resolveMediaSendParams: %v", err)
	}

	if receivedMethod != "/cloud/cdnBigUpload" {
		t.Fatalf("method = %q", receivedMethod)
	}
	if receivedGUID != "guid-1" || receivedToken != "token-1" {
		t.Fatalf("auth fields guid=%q token=%q", receivedGUID, receivedToken)
	}
	if receivedFileType != "5" {
		t.Fatalf("fileType = %q", receivedFileType)
	}
	if receivedFilename != "office-proof.docx" {
		t.Fatalf("filename = %q", receivedFilename)
	}
	if receivedBody != "docx-content" {
		t.Fatalf("body = %q", receivedBody)
	}
	if method != "/msg/sendFile" {
		t.Fatalf("send method = %q", method)
	}
	if params["toId"] != "user-1" || params["fileId"] != "file-id" || params["filename"] != "office-proof.docx" {
		t.Fatalf("unexpected send params: %+v", params)
	}
}

func TestResolveMediaSendParamsFallsBackToURLUpload(t *testing.T) {
	var receivedBody string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		_ = json.NewEncoder(w).Encode(qiweiDoAPIResponse{
			Code: 0,
			Msg:  "成功",
			Data: json.RawMessage(`{"fileAesKey":"aes","fileId":"id","fileSize":1,"filename":"fallback.pdf"}`),
		})
	}))
	defer api.Close()

	app := &app{http: &http.Client{Timeout: time.Millisecond}}
	rt := newAccountRuntime(Config{APIBaseURL: api.URL, RequestTimout: 5}, nil, Account{
		ID:      "qw_test",
		GUID:    "guid-1",
		Token:   "token-1",
		Enabled: true,
	})

	method, _, err := app.resolveMediaSendParams(context.Background(), rt, "file", "user-1", "http://127.0.0.1:1/file.pdf", nil)
	if err != nil {
		t.Fatalf("resolveMediaSendParams fallback: %v", err)
	}
	if method != "/msg/sendFile" {
		t.Fatalf("method = %q", method)
	}
	if !strings.Contains(receivedBody, `"/cloud/cdnBigUploadByUrl"`) || !strings.Contains(receivedBody, `"fileUrl"`) {
		t.Fatalf("fallback body = %s", receivedBody)
	}
}

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
