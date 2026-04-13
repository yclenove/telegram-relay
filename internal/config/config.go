package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Security SecurityConfig `yaml:"security"`
	Telegram TelegramConfig `yaml:"telegram"`
	Retry    RetryConfig    `yaml:"retry"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Worker   WorkerConfig   `yaml:"worker"`
	Web      WebConfig      `yaml:"web"`
}

// ServerConfig 描述 HTTP 服务监听参数。
type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
}

// SecurityConfig 描述入站安全策略。
// level:
// - basic: 只校验 Bearer Token
// - medium: Token + 时间戳 + HMAC 签名
// - strict: medium + IP 白名单
type SecurityConfig struct {
	Level              string   `yaml:"level"`
	Token              string   `yaml:"token"`
	HMACSecret         string   `yaml:"hmac_secret"`
	TimestampSkewSec   int64    `yaml:"timestamp_skew_sec"`
	IPWhitelist        []string `yaml:"ip_whitelist"`
	RateLimitPerSecond float64  `yaml:"rate_limit_per_second"`
	RateLimitBurst     int      `yaml:"rate_limit_burst"`
}

// TelegramConfig 描述 Telegram Bot API 连接参数。
type TelegramConfig struct {
	BotToken   string `yaml:"bot_token"`
	ChatID     string `yaml:"chat_id"`
	ParseMode  string `yaml:"parse_mode"`
	APIBaseURL string `yaml:"api_base_url"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

// RetryConfig 描述 Telegram 发送失败后的重试策略。
type RetryConfig struct {
	MaxAttempts       int `yaml:"max_attempts"`
	InitialBackoffMS  int `yaml:"initial_backoff_ms"`
	MaxBackoffMS      int `yaml:"max_backoff_ms"`
}

// DatabaseConfig 描述 PostgreSQL 连接参数。
type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

// AuthConfig 描述管理端认证与 Token 配置。
type AuthConfig struct {
	JWTSecret          string `yaml:"jwt_secret"`
	AccessTokenTTLMin  int    `yaml:"access_token_ttl_min"`
	RefreshTokenTTLMin int    `yaml:"refresh_token_ttl_min"`
	BootstrapUsername  string `yaml:"bootstrap_username"`
	BootstrapPassword  string `yaml:"bootstrap_password"`
}

// WorkerConfig 描述异步分发 worker 的轮询与批次参数。
type WorkerConfig struct {
	PollIntervalMS int `yaml:"poll_interval_ms"`
	BatchSize      int `yaml:"batch_size"`
}

// WebConfig 描述前端静态资源路径与控制参数。
type WebConfig struct {
	AdminStaticDir string `yaml:"admin_static_dir"`
}

// Load 加载配置：
// 1) 先加载默认值
// 2) 可选读取 CONFIG_FILE 指向的 YAML
// 3) 使用环境变量覆盖
// 4) 执行校验
func Load() (Config, error) {
	cfg := defaultConfig()

	// 兼容单文件模式：CONFIG_FILE
	if path := os.Getenv("CONFIG_FILE"); path != "" {
		if err := mergeYAML(path, &cfg); err != nil {
			return cfg, err
		}
	}
	// 推荐分层模式：公开配置 + 私密配置（私密文件不入库）。
	if path := os.Getenv("CONFIG_PUBLIC_FILE"); path != "" {
		if err := mergeYAML(path, &cfg); err != nil {
			return cfg, err
		}
	}
	if path := os.Getenv("CONFIG_PRIVATE_FILE"); path != "" {
		if err := mergeYAML(path, &cfg); err != nil {
			return cfg, err
		}
	}

	overrideFromEnv(&cfg)
	// 与 MCP 等工具共用 PG_* 分项时，避免重复维护一整条 DATABASE_DSN。
	applyDatabaseDSNFromPGEnv(&cfg)
	if err := validate(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// defaultConfig 提供系统可运行的默认参数（但不包含敏感字段）。
func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			ListenAddr: ":8080",
		},
		Security: SecurityConfig{
			Level:              "basic",
			TimestampSkewSec:   300,
			RateLimitPerSecond: 20,
			RateLimitBurst:     40,
		},
		Telegram: TelegramConfig{
			ParseMode:  "HTML",
			APIBaseURL: "https://api.telegram.org",
			TimeoutSec: 5,
		},
		Retry: RetryConfig{
			MaxAttempts:      3,
			InitialBackoffMS: 300,
			MaxBackoffMS:     3000,
		},
		Database: DatabaseConfig{
			DSN: "",
		},
		Auth: AuthConfig{
			AccessTokenTTLMin:  60,
			RefreshTokenTTLMin: 24 * 60,
			BootstrapUsername:  "admin",
		},
		Worker: WorkerConfig{
			PollIntervalMS: 1000,
			BatchSize:      20,
		},
		// 管理台为独立仓库构建产物时，通过环境变量或配置指向 dist 目录；留空表示不挂载静态站点（仅 API）。
		Web: WebConfig{
			AdminStaticDir: "",
		},
	}
}

// overrideFromEnv 从环境变量读取参数并覆盖配置文件中的值。
// 设计为静默忽略非法值，避免因为单个配置项格式错误导致服务无法启动。
func overrideFromEnv(cfg *Config) {
	setString := func(env string, dst *string) {
		if v := os.Getenv(env); v != "" {
			*dst = v
		}
	}
	setInt := func(env string, dst *int) {
		if v := os.Getenv(env); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	setInt64 := func(env string, dst *int64) {
		if v := os.Getenv(env); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				*dst = n
			}
		}
	}
	setFloat := func(env string, dst *float64) {
		if v := os.Getenv(env); v != "" {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				*dst = n
			}
		}
	}

	setString("LISTEN_ADDR", &cfg.Server.ListenAddr)
	setString("SECURITY_LEVEL", &cfg.Security.Level)
	setString("AUTH_TOKEN", &cfg.Security.Token)
	setString("HMAC_SECRET", &cfg.Security.HMACSecret)
	setInt64("TIMESTAMP_SKEW_SEC", &cfg.Security.TimestampSkewSec)
	if wl := os.Getenv("IP_WHITELIST"); wl != "" {
		cfg.Security.IPWhitelist = splitCSV(wl)
	}
	setFloat("RATE_LIMIT_PER_SECOND", &cfg.Security.RateLimitPerSecond)
	setInt("RATE_LIMIT_BURST", &cfg.Security.RateLimitBurst)

	setString("TELEGRAM_BOT_TOKEN", &cfg.Telegram.BotToken)
	setString("TELEGRAM_CHAT_ID", &cfg.Telegram.ChatID)
	setString("TELEGRAM_PARSE_MODE", &cfg.Telegram.ParseMode)
	setString("TELEGRAM_API_BASE_URL", &cfg.Telegram.APIBaseURL)
	setInt("TELEGRAM_TIMEOUT_SEC", &cfg.Telegram.TimeoutSec)

	setInt("RETRY_MAX_ATTEMPTS", &cfg.Retry.MaxAttempts)
	setInt("RETRY_INITIAL_BACKOFF_MS", &cfg.Retry.InitialBackoffMS)
	setInt("RETRY_MAX_BACKOFF_MS", &cfg.Retry.MaxBackoffMS)

	setString("DATABASE_DSN", &cfg.Database.DSN)
	setString("JWT_SECRET", &cfg.Auth.JWTSecret)
	setInt("ACCESS_TOKEN_TTL_MIN", &cfg.Auth.AccessTokenTTLMin)
	setInt("REFRESH_TOKEN_TTL_MIN", &cfg.Auth.RefreshTokenTTLMin)
	setString("BOOTSTRAP_USERNAME", &cfg.Auth.BootstrapUsername)
	setString("BOOTSTRAP_PASSWORD", &cfg.Auth.BootstrapPassword)
	setInt("WORKER_POLL_INTERVAL_MS", &cfg.Worker.PollIntervalMS)
	setInt("WORKER_BATCH_SIZE", &cfg.Worker.BatchSize)
	setString("ADMIN_STATIC_DIR", &cfg.Web.AdminStaticDir)
}

// applyDatabaseDSNFromPGEnv 在未设置 DATABASE_DSN 时，用 PG_HOST/PG_USER/PG_PASSWORD 等拼出 libpq 风格连接串。
// 密码中的特殊字符由 net/url.UserPassword 编码，避免手写 DSN 出错；仅当 PG_HOST 与 PG_USER 均非空时才生效。
func applyDatabaseDSNFromPGEnv(cfg *Config) {
	if strings.TrimSpace(cfg.Database.DSN) != "" {
		return
	}
	host := strings.TrimSpace(os.Getenv("PG_HOST"))
	user := strings.TrimSpace(os.Getenv("PG_USER"))
	pass := os.Getenv("PG_PASSWORD")
	if host == "" || user == "" {
		return
	}
	port := strings.TrimSpace(os.Getenv("PG_PORT"))
	if port == "" {
		port = "5432"
	}
	db := strings.TrimSpace(os.Getenv("PG_DATABASE"))
	if db == "" {
		db = "telegram"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + db,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	cfg.Database.DSN = u.String()
}

// splitCSV 将逗号分隔字符串转换为切片，用于白名单等配置项。
func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// validate 对最终配置做强校验，尽可能在启动阶段就暴露问题。
func validate(cfg Config) error {
	switch cfg.Security.Level {
	case "basic", "medium", "strict":
	default:
		return fmt.Errorf("unsupported security level: %s", cfg.Security.Level)
	}
	if cfg.Security.Token == "" {
		return errors.New("missing auth token：请设置环境变量 AUTH_TOKEN，或在私密 YAML 中配置 security.token；仓库根目录的 .env 会在启动时自动加载（若存在）")
	}
	if cfg.Security.Level != "basic" && cfg.Security.HMACSecret == "" {
		return errors.New("missing HMAC secret for medium/strict mode")
	}
	if cfg.Telegram.BotToken == "" || cfg.Telegram.ChatID == "" {
		return errors.New("missing telegram bot token or chat id")
	}
	if cfg.Security.TimestampSkewSec <= 0 {
		return errors.New("timestamp skew must be > 0")
	}
	if cfg.Telegram.TimeoutSec <= 0 {
		return errors.New("telegram timeout must be > 0")
	}
	if cfg.Retry.MaxAttempts <= 0 {
		return errors.New("retry max attempts must be > 0")
	}
	if cfg.Retry.InitialBackoffMS <= 0 || cfg.Retry.MaxBackoffMS <= 0 {
		return errors.New("retry backoff must be > 0")
	}
	if cfg.Retry.InitialBackoffMS > cfg.Retry.MaxBackoffMS {
		return errors.New("retry initial backoff cannot exceed max backoff")
	}
	if cfg.Security.RateLimitPerSecond <= 0 || cfg.Security.RateLimitBurst <= 0 {
		return errors.New("rate limit must be > 0")
	}
	if cfg.Database.DSN == "" {
		return errors.New("missing database dsn")
	}
	if cfg.Auth.JWTSecret == "" {
		return errors.New("missing jwt secret")
	}
	if cfg.Auth.BootstrapUsername == "" || cfg.Auth.BootstrapPassword == "" {
		return errors.New("missing bootstrap admin credentials")
	}
	if cfg.Auth.AccessTokenTTLMin <= 0 || cfg.Auth.RefreshTokenTTLMin <= 0 {
		return errors.New("auth token ttl must be > 0")
	}
	if cfg.Worker.PollIntervalMS <= 0 || cfg.Worker.BatchSize <= 0 {
		return errors.New("worker configuration must be > 0")
	}
	if _, err := time.ParseDuration(fmt.Sprintf("%ds", cfg.Telegram.TimeoutSec)); err != nil {
		return fmt.Errorf("invalid telegram timeout: %w", err)
	}
	return nil
}

func mergeYAML(path string, cfg *Config) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", filepath.Clean(path), err)
	}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return fmt.Errorf("parse config file %s: %w", filepath.Clean(path), err)
	}
	return nil
}
