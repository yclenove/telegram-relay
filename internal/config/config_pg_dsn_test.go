package config

import (
	"net/url"
	"strings"
	"testing"
)

// TestApplyDatabaseDSNFromPGEnv 验证未设 DATABASE_DSN 时能从 PG_* 拼出带编码的连接串，
// 避免密码含 @、: 等字符时手工拼 URL 出错。
func TestApplyDatabaseDSNFromPGEnv(t *testing.T) {
	t.Setenv("DATABASE_DSN", "")
	t.Setenv("PG_HOST", "127.0.0.1")
	t.Setenv("PG_PORT", "5433")
	t.Setenv("PG_USER", "app")
	t.Setenv("PG_PASSWORD", "p@:x")
	t.Setenv("PG_DATABASE", "telegram")

	cfg := defaultConfig()
	overrideFromEnv(&cfg)
	applyDatabaseDSNFromPGEnv(&cfg)

	if cfg.Database.DSN == "" {
		t.Fatal("expected non-empty DSN")
	}
	if !strings.Contains(cfg.Database.DSN, "127.0.0.1:5433") {
		t.Fatalf("unexpected host/port in DSN: %q", cfg.Database.DSN)
	}
	if !strings.Contains(cfg.Database.DSN, "/telegram") {
		t.Fatalf("unexpected db name in DSN: %q", cfg.Database.DSN)
	}
	u, err := url.Parse(cfg.Database.DSN)
	if err != nil {
		t.Fatal(err)
	}
	if u.User.Username() != "app" {
		t.Fatalf("user: %v", u.User.Username())
	}
	pw, _ := u.User.Password()
	if pw != "p@:x" {
		t.Fatalf("password decode: want p@:x got %q", pw)
	}
}
