package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yclenove/telegram-relay/internal/config"
)

func TestHashPasswordIsBcrypt(t *testing.T) {
	t.Parallel()
	a, err := HashPassword("abc123")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("bcrypt hashes should differ across salts")
	}
	if !VerifyPassword(a, "abc123") || !VerifyPassword(b, "abc123") {
		t.Fatal("bcrypt verify failed")
	}
	if VerifyPassword(a, "wrong") {
		t.Fatal("bcrypt should reject wrong password")
	}
}

func TestVerifyLegacySHA256Hex(t *testing.T) {
	t.Parallel()
	legacy := hashSHA256Legacy("old-secret")
	if !VerifyPassword(legacy, "old-secret") {
		t.Fatal("legacy sha256 verify failed")
	}
	if VerifyPassword(legacy, "nope") {
		t.Fatal("legacy should reject wrong password")
	}
}

func TestParseToken(t *testing.T) {
	t.Parallel()
	svc := NewAuthService(nil, config.AuthConfig{JWTSecret: "unit-secret", AccessTokenTTLMin: 60, RefreshTokenTTLMin: 1440})
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ":   jwtTypAccess,
		"uid":   float64(7),
		"perms": []string{"bot.manage", "rule.manage"},
		"exp":   time.Now().Add(time.Minute).Unix(),
	})
	raw, err := token.SignedString([]byte("unit-secret"))
	if err != nil {
		t.Fatalf("sign token failed: %v", err)
	}
	uid, perms, err := svc.ParseToken(raw)
	if err != nil {
		t.Fatalf("parse token failed: %v", err)
	}
	if uid != 7 {
		t.Fatalf("unexpected uid: %d", uid)
	}
	if !perms["bot.manage"] || !perms["rule.manage"] {
		t.Fatalf("permissions should be present")
	}
}

func TestParseTokenRejectsRefreshJWT(t *testing.T) {
	t.Parallel()
	svc := NewAuthService(nil, config.AuthConfig{JWTSecret: "unit-secret", AccessTokenTTLMin: 60, RefreshTokenTTLMin: 1440})
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ": jwtTypRefresh,
		"uid": float64(1),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	raw, err := token.SignedString([]byte("unit-secret"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.ParseToken(raw)
	if err == nil {
		t.Fatal("expected refresh token rejected by ParseToken")
	}
}
