package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/m-guo-2/ouroboros-agent/shared/oss"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port       string `yaml:"port"`
	Version    string `yaml:"version"`
	ID         string `yaml:"id"`
	LogDir     string `yaml:"log_dir"`
	DBPath     string `yaml:"db_path"`
	AdminDist  string `yaml:"admin_dist"`
	Qiwei      Qiwei  `yaml:"qiwei"`
	GitHub     GitHub `yaml:"github"`
	OSS        OSS    `yaml:"oss"`
	ConfigPath string `yaml:"-"`
}

type Qiwei struct {
	BaseURL string `yaml:"base_url"`
}

type OSS struct {
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
	Prefix    string `yaml:"prefix"`
	UseSSL    bool   `yaml:"use_ssl"`
}

// ToSharedConfig converts to the shared oss.Config used by storage clients.
func (o OSS) ToSharedConfig() oss.Config {
	return oss.Config{
		Endpoint:  o.Endpoint,
		Bucket:    o.Bucket,
		AccessKey: o.AccessKey,
		SecretKey: o.SecretKey,
		Region:    o.Region,
		Prefix:    o.Prefix,
		UseSSL:    o.UseSSL,
	}
}

type GitHub struct {
	Token          string `yaml:"token"`
	SkillsRepo     string `yaml:"skills_repo"`
	Branch         string `yaml:"branch"`
	SkillsPath     string `yaml:"skills_path"`
	SkillsLocalDir string `yaml:"skills_local_dir"` // local disk cache for skill files (scripts, references)
	SyncInterval   string `yaml:"sync_interval"`
}

// ParseSyncInterval returns the sync interval as a time.Duration.
// Defaults to 15 minutes if not set or invalid.
func (g GitHub) ParseSyncInterval() time.Duration {
	if g.SyncInterval == "" {
		return 15 * time.Minute
	}
	d, err := time.ParseDuration(g.SyncInterval)
	if err != nil || d <= 0 {
		return 15 * time.Minute
	}
	return d
}

func defaults() Config {
	return Config{
		Port:      "1997",
		Version:   "1.0.0",
		ID:        "agent-instance",
		LogDir:    filepath.Join("data", "logs"),
		DBPath:    filepath.Join("data", "config.db"),
		AdminDist: "",
		GitHub: GitHub{
			Branch:     "main",
			SkillsPath: "skills",
		},
	}
}

// Load parses the -config flag, reads the YAML file, and returns a Config.
// Search order for the config file:
//  1. -config flag
//  2. ./config.yaml
//  3. ../config.yaml
func Load() (Config, error) {
	var configPath string
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.Parse()

	cfg := defaults()

	if configPath == "" {
		for _, candidate := range []string{"config.yaml", "../config.yaml"} {
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}
	}

	if configPath == "" {
		applyEnvOverrides(&cfg)
		current = cfg
		return cfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("读取配置文件 %s: %w", configPath, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析配置文件 %s: %w", configPath, err)
	}

	cfg.ConfigPath = configPath
	applyEnvOverrides(&cfg)
	current = cfg
	return cfg, nil
}

var current Config

func Current() Config {
	return current
}

func ResolveQiweiBaseURL(getSetting func(string) string) string {
	if baseURL := normalizeBaseURL(Current().Qiwei.BaseURL); baseURL != "" {
		return baseURL
	}

	port := strings.TrimSpace(getSetting("general.qiwei_port"))
	if port == "" {
		port = "2000"
	}
	return fmt.Sprintf("http://localhost:%s", port)
}

func normalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// applyEnvOverrides lets environment variables override YAML values.
// Only non-empty env vars take effect, so YAML remains the default source.
func applyEnvOverrides(cfg *Config) {
	envStr := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	envStr("PORT", &cfg.Port)
	envStr("AGENT_APP_VERSION", &cfg.Version)
	envStr("AGENT_ID", &cfg.ID)
	envStr("DB_PATH", &cfg.DBPath)
	envStr("LOG_DIR", &cfg.LogDir)
	envStr("ADMIN_DIST", &cfg.AdminDist)

	envStr("GITHUB_TOKEN", &cfg.GitHub.Token)
	envStr("GITHUB_SKILLS_REPO", &cfg.GitHub.SkillsRepo)
	envStr("GITHUB_SKILLS_BRANCH", &cfg.GitHub.Branch)
	envStr("GITHUB_SKILLS_PATH", &cfg.GitHub.SkillsPath)
	envStr("GITHUB_SKILLS_LOCAL_DIR", &cfg.GitHub.SkillsLocalDir)
	envStr("GITHUB_SYNC_INTERVAL", &cfg.GitHub.SyncInterval)

	envStr("OSS_ENDPOINT", &cfg.OSS.Endpoint)
	envStr("OSS_BUCKET", &cfg.OSS.Bucket)
	envStr("OSS_ACCESS_KEY", &cfg.OSS.AccessKey)
	envStr("OSS_SECRET_KEY", &cfg.OSS.SecretKey)
	envStr("OSS_REGION", &cfg.OSS.Region)
	envStr("OSS_PREFIX", &cfg.OSS.Prefix)
	envBool := func(key string, dst *bool) {
		if v := os.Getenv(key); v != "" {
			*dst = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
		}
	}
	envBool("OSS_USE_SSL", &cfg.OSS.UseSSL)
}
