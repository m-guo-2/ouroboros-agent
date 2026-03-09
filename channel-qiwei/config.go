package main

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrInvalidConfig = errors.New("invalid qiwei config")

type Config struct {
	APIBaseURL    string
	Token         string
	GUID          string
	Port          string
	AgentEnabled  bool
	AgentServer   string
	AgentID       string
	LogLevel      string
	RequestTimout int
}

func LoadConfig() Config {
	loadDotEnv("../.env")
	loadDotEnv(".env")

	agentEnabled := strings.ToLower(getenv("AGENT_ENABLED", "true")) != "false"

	return Config{
		APIBaseURL:    strings.TrimRight(getenv("QIWEI_API_BASE_URL", "http://manager.qiweapi.com/qiwe"), "/"),
		Token:         getenv("QIWEI_TOKEN", ""),
		GUID:          getenv("QIWEI_GUID", ""),
		Port:          getenv("QIWEI_BOT_PORT", "2000"),
		AgentEnabled:  agentEnabled,
		AgentServer:   strings.TrimRight(getenv("AGENT_SERVER_URL", "http://localhost:1997"), "/"),
		AgentID:       getenv("AGENT_ID", ""),
		LogLevel:      strings.ToLower(getenv("QIWEI_LOG_LEVEL", "info")),
		RequestTimout: parseIntOrDefault(getenv("QIWEI_HTTP_TIMEOUT_SECONDS", "25"), 25),
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.APIBaseURL) == "" || strings.TrimSpace(c.Token) == "" || strings.TrimSpace(c.GUID) == "" {
		return ErrInvalidConfig
	}
	return nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadDotEnv(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	file, err := os.Open(abs)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
