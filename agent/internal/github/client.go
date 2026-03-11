package github

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agent/internal/config"
)

type Client struct {
	token  string
	owner  string
	repo   string
	branch string
	http   *http.Client
}

type FileEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	SHA  string `json:"sha"`
	Type string `json:"type"` // "file" or "dir"
	Size int    `json:"size"`
}

type FileContent struct {
	FileEntry
	Content  string `json:"content"`  // base64
	Encoding string `json:"encoding"` // "base64"
}

type writeRequest struct {
	Message string `json:"message"`
	Content string `json:"content,omitempty"` // base64
	SHA     string `json:"sha,omitempty"`
	Branch  string `json:"branch,omitempty"`
}

type deleteRequest struct {
	Message string `json:"message"`
	SHA     string `json:"sha"`
	Branch  string `json:"branch,omitempty"`
}

type apiError struct {
	StatusCode int
	Message    string `json:"message"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("github api %d: %s", e.StatusCode, e.Message)
}

func NewClient(token, owner, repo, branch string) *Client {
	return &Client{
		token:  token,
		owner:  owner,
		repo:   repo,
		branch: branch,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// NewClientFromConfig creates a Client from the GitHub config section.
func NewClientFromConfig(gh config.GitHub) (*Client, error) {
	if gh.Token == "" {
		return nil, fmt.Errorf("github.token not set")
	}
	if gh.SkillsRepo == "" {
		return nil, fmt.Errorf("github.skills_repo not set")
	}
	parts := strings.SplitN(gh.SkillsRepo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("github.skills_repo must be owner/repo, got %q", gh.SkillsRepo)
	}
	branch := gh.Branch
	if branch == "" {
		branch = "main"
	}
	return NewClient(gh.Token, parts[0], parts[1], branch), nil
}

func (c *Client) contentsURL(path string) string {
	path = strings.TrimPrefix(path, "/")
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", c.owner, c.repo, path)
}

func (c *Client) do(method, url string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	ae := &apiError{StatusCode: resp.StatusCode, Message: string(data)}
	_ = json.Unmarshal(data, ae)
	return ae
}

// ListDir lists entries in a directory. Returns ErrNotFound for 404.
func (c *Client) ListDir(path string) ([]FileEntry, error) {
	url := c.contentsURL(path)
	if c.branch != "" {
		url += "?ref=" + c.branch
	}
	resp, err := c.do("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	var entries []FileEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode dir listing: %w", err)
	}
	return entries, nil
}

// GetFile retrieves a single file's content (base64-encoded in FileContent.Content).
func (c *Client) GetFile(path string) (*FileContent, error) {
	url := c.contentsURL(path)
	if c.branch != "" {
		url += "?ref=" + c.branch
	}
	resp, err := c.do("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	var fc FileContent
	if err := json.NewDecoder(resp.Body).Decode(&fc); err != nil {
		return nil, fmt.Errorf("decode file: %w", err)
	}
	return &fc, nil
}

// GetFileContent is a convenience wrapper: returns decoded (UTF-8) content and SHA.
func (c *Client) GetFileContent(path string) (content string, sha string, err error) {
	fc, err := c.GetFile(path)
	if err != nil {
		return "", "", err
	}
	raw := strings.ReplaceAll(fc.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", "", fmt.Errorf("decode base64: %w", err)
	}
	return string(decoded), fc.SHA, nil
}

// PutFile creates or updates a file. Pass sha="" for creation.
func (c *Client) PutFile(path, message, content, sha string) error {
	body := writeRequest{
		Message: message,
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
		Branch:  c.branch,
	}
	if sha != "" {
		body.SHA = sha
	}
	resp, err := c.do("PUT", c.contentsURL(path), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkResponse(resp)
}

// DeleteFile removes a file. SHA is required.
func (c *Client) DeleteFile(path, message, sha string) error {
	body := deleteRequest{
		Message: message,
		SHA:     sha,
		Branch:  c.branch,
	}
	resp, err := c.do("DELETE", c.contentsURL(path), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkResponse(resp)
}

// IsNotFound returns true if the error is a 404.
func IsNotFound(err error) bool {
	ae, ok := err.(*apiError)
	return ok && ae.StatusCode == 404
}
