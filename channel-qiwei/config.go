package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var ErrInvalidConfig = errors.New("invalid qiwei config")

type Config struct {
	APIBaseURL  string `yaml:"api_base_url"`
	Token       string `yaml:"token"`
	GUID        string `yaml:"guid"`
	Port        string `yaml:"port"`
	LogLevel    string `yaml:"log_level"`
	LogDir      string `yaml:"log_dir"`
	DataDir     string `yaml:"data_dir"`
	HTTPTimeout int    `yaml:"http_timeout"`

	// DBPath is the SQLite file holding the multi-account registry.
	// Defaults to "{DataDir}/qiwei.db" when empty.
	DBPath string `yaml:"db_path"`

	// AdminToken gates the /api/qiwei/_admin/* endpoints. When empty the
	// admin router is not mounted at all (a safer default than "any
	// caller wins").
	AdminToken string `yaml:"admin_token"`

	// ProfileSyncInterval is how often background goroutines refresh each
	// account's identity snapshot. Defaults to 6h when zero.
	ProfileSyncInterval time.Duration `yaml:"profile_sync_interval"`

	// ContactSyncInterval is the full-sync cadence for contact/room snapshots.
	// Defaults to 6h when zero.
	ContactSyncInterval time.Duration `yaml:"contact_sync_interval"`

	// ContactSyncEnabled controls whether the periodic full sync loop runs.
	// Event-driven enqueue remains available even when this is false.
	ContactSyncEnabled bool `yaml:"contact_sync_enabled"`

	// GatewayToken gates the /api/qiwei/gateway/* endpoints. Empty means the
	// gateway HTTP API is not mounted.
	GatewayToken string `yaml:"gateway_token"`

	Agent AgentConfig `yaml:"agent"`
	Volc  VolcConfig  `yaml:"volc"`
	OSS   OSSConfig   `yaml:"oss"`

	// 保留旧字段兼容，由 YAML 解析后填充
	AgentEnabled         bool   `yaml:"-"`
	AgentServer          string `yaml:"-"`
	AgentID              string `yaml:"-"`
	RequestTimout        int    `yaml:"-"`
	VolcArkBaseURL       string `yaml:"-"`
	VolcArkAPIKey        string `yaml:"-"`
	VolcVisionModel      string `yaml:"-"`
	VolcDocumentModel    string `yaml:"-"`
	VolcSpeechAppKey     string `yaml:"-"`
	VolcSpeechAccessKey  string `yaml:"-"`
	VolcSpeechResourceID string `yaml:"-"`
	VolcSpeechSubmitURL  string `yaml:"-"`
	VolcSpeechQueryURL   string `yaml:"-"`
	OSSPublicBaseURL     string `yaml:"-"`
}

type AgentConfig struct {
	Enabled   bool   `yaml:"enabled"`
	ServerURL string `yaml:"server_url"`
	ID        string `yaml:"id"`
}

type VolcConfig struct {
	Ark    VolcArkConfig    `yaml:"ark"`
	Speech VolcSpeechConfig `yaml:"speech"`
}

type VolcArkConfig struct {
	BaseURL       string `yaml:"base_url"`
	APIKey        string `yaml:"api_key"`
	VisionModel   string `yaml:"vision_model"`
	DocumentModel string `yaml:"document_model"`
}

type VolcSpeechConfig struct {
	AppKey     string `yaml:"app_key"`
	AccessKey  string `yaml:"access_key"`
	ResourceID string `yaml:"resource_id"`
	SubmitURL  string `yaml:"submit_url"`
	QueryURL   string `yaml:"query_url"`
}

type OSSConfig struct {
	Endpoint      string `yaml:"endpoint"`
	Bucket        string `yaml:"bucket"`
	AccessKey     string `yaml:"access_key"`
	SecretKey     string `yaml:"secret_key"`
	Region        string `yaml:"region"`
	Prefix        string `yaml:"prefix"`
	UseSSL        bool   `yaml:"use_ssl"`
	PublicBaseURL string `yaml:"public_base_url"`
}

func LoadConfig() Config {
	var configPath string
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.Parse()

	cfg := configDefaults()

	if configPath == "" {
		for _, c := range []string{"config.yaml", "../config.yaml"} {
			if _, err := os.Stat(c); err == nil {
				configPath = c
				break
			}
		}
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取配置文件失败: %v\n", err)
			os.Exit(1)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "解析配置文件失败: %v\n", err)
			os.Exit(1)
		}
	}

	cfg.flatten()
	return cfg
}

func configDefaults() Config {
	return Config{
		APIBaseURL:          "http://manager.qiweapi.com/qiwe",
		Port:                "2000",
		LogLevel:            "info",
		DataDir:             "./data",
		HTTPTimeout:         25,
		ProfileSyncInterval: 6 * time.Hour,
		ContactSyncInterval: 6 * time.Hour,
		ContactSyncEnabled:  true,
		Agent: AgentConfig{
			Enabled:   true,
			ServerURL: "http://localhost:1997",
		},
		Volc: VolcConfig{
			Ark: VolcArkConfig{
				BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
			},
			Speech: VolcSpeechConfig{
				SubmitURL: "https://openspeech.bytedance.com/api/v3/auc/bigmodel/submit",
				QueryURL:  "https://openspeech.bytedance.com/api/v3/auc/bigmodel/query",
			},
		},
	}
}

// flatten 将嵌套结构铺平到旧字段，保持下游代码兼容
func (c *Config) flatten() {
	c.APIBaseURL = strings.TrimRight(c.APIBaseURL, "/")
	c.LogLevel = strings.ToLower(c.LogLevel)
	if strings.TrimSpace(c.DBPath) == "" {
		c.DBPath = filepath.Join(c.DataDir, "qiwei.db")
	}
	if c.ProfileSyncInterval <= 0 {
		c.ProfileSyncInterval = 6 * time.Hour
	}
	if c.ContactSyncInterval <= 0 {
		c.ContactSyncInterval = 6 * time.Hour
	}

	c.AgentEnabled = c.Agent.Enabled
	c.AgentServer = strings.TrimRight(c.Agent.ServerURL, "/")
	c.AgentID = c.Agent.ID
	c.RequestTimout = c.HTTPTimeout

	c.VolcArkBaseURL = strings.TrimRight(c.Volc.Ark.BaseURL, "/")
	c.VolcArkAPIKey = c.Volc.Ark.APIKey
	c.VolcVisionModel = c.Volc.Ark.VisionModel
	c.VolcDocumentModel = c.Volc.Ark.DocumentModel

	c.VolcSpeechAppKey = c.Volc.Speech.AppKey
	c.VolcSpeechAccessKey = c.Volc.Speech.AccessKey
	c.VolcSpeechResourceID = c.Volc.Speech.ResourceID
	c.VolcSpeechSubmitURL = strings.TrimRight(c.Volc.Speech.SubmitURL, "/")
	c.VolcSpeechQueryURL = strings.TrimRight(c.Volc.Speech.QueryURL, "/")

	c.OSSPublicBaseURL = strings.TrimRight(c.OSS.PublicBaseURL, "/")
}

// Validate enforces the minimum platform-level config. With multi-account
// support the historical requirement "top-level guid/token must be set" is
// gone: an empty YAML is fine as long as at least one account exists in the
// DB (checked separately by the runtime at boot).
func (c Config) Validate() error {
	if strings.TrimSpace(c.APIBaseURL) == "" {
		return fmt.Errorf("%w: api_base_url is required", ErrInvalidConfig)
	}
	return nil
}
