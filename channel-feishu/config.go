package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AppID             string
	AppSecret         string
	EncryptKey        string
	VerificationToken string
	Port              string
	LogLevel          string
	AgentEnabled      bool
	AgentServerURL    string
	AgentID           string
	Env               string
}

func LoadConfig() Config {
	loadDotEnv("../.env")
	loadDotEnv(".env")

	port := getenv("FEISHU_BOT_PORT", "1999")
	logLevel := strings.ToLower(getenv("FEISHU_LOG_LEVEL", "info"))
	agentEnabled := strings.ToLower(getenv("AGENT_ENABLED", "true")) != "false"

	return Config{
		AppID:             getenv("FEISHU_APP_ID", ""),
		AppSecret:         getenv("FEISHU_APP_SECRET", ""),
		EncryptKey:        getenv("FEISHU_ENCRYPT_KEY", ""),
		VerificationToken: getenv("FEISHU_VERIFICATION_TOKEN", ""),
		Port:              port,
		LogLevel:          logLevel,
		AgentEnabled:      agentEnabled,
		AgentServerURL:    strings.TrimRight(getenv("AGENT_SERVER_URL", "http://localhost:1997"), "/"),
		AgentID:           getenv("AGENT_ID", ""),
		Env:               getenv("NODE_ENV", "development"),
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.AppID) == "" || strings.TrimSpace(c.AppSecret) == "" {
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

func parseIntOrDefault(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
