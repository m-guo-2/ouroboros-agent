package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore writes logs as JSONL files, preserving the original on-disk format.
// It implements LogWriter only; reading is handled by SQLiteStore.
type FileStore struct {
	dir   string
	mu    sync.Mutex
	files map[string]*os.File
}

func NewFileStore(dir string) (*FileStore, error) {
	for _, level := range []Level{LBoundary, LBusiness, LDetail} {
		if err := os.MkdirAll(filepath.Join(dir, string(level)), 0755); err != nil {
			return nil, fmt.Errorf("create log dir %s: %w", level, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "detail", "llm-io"), 0755); err != nil {
		return nil, fmt.Errorf("create llm-io dir: %w", err)
	}
	return &FileStore{dir: dir, files: make(map[string]*os.File)}, nil
}

func (fs *FileStore) Append(level Level, entry map[string]any) error {
	ts, _ := entry["time"].(string)
	date := time.Now().Format("2006-01-02")
	if len(ts) >= 10 {
		date = ts[:10]
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	key := string(level) + "/" + date

	fs.mu.Lock()
	defer fs.mu.Unlock()

	f, ok := fs.files[key]
	if !ok {
		path := filepath.Join(fs.dir, key+".jsonl")
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		fs.files[key] = f
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	_, err = f.Write([]byte("\n"))
	return err
}

func (fs *FileStore) WriteLLMIO(ref string, _ string, _ int, data []byte) error {
	path := filepath.Join(fs.dir, "detail", "llm-io", ref+".json")
	return os.WriteFile(path, data, 0644)
}

func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for k, f := range fs.files {
		_ = f.Close()
		delete(fs.files, k)
	}
	return nil
}
