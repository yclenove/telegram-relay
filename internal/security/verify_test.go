package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"telegram-notification/internal/config"
)

func TestVerifyBasicSuccess(t *testing.T) {
	t.Parallel()

	v := NewVerifier(config.SecurityConfig{
		Level: "basic",
		Token: "token-123",
	})
	req := httptest.NewRequest("POST", "/api/v1/notify", nil)
	req.Header.Set("Authorization", "Bearer token-123")

	if err := v.VerifyRequest(req, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestVerifyBasicInvalidToken(t *testing.T) {
	t.Parallel()

	v := NewVerifier(config.SecurityConfig{
		Level: "basic",
		Token: "token-123",
	})
	req := httptest.NewRequest("POST", "/api/v1/notify", nil)
	req.Header.Set("Authorization", "Bearer bad-token")

	if err := v.VerifyRequest(req, []byte(`{"a":1}`)); err == nil {
		t.Fatalf("expected error for invalid token")
	}
}

func TestVerifyMediumSuccess(t *testing.T) {
	t.Parallel()

	secret := "hmac-secret"
	body := []byte(`{"title":"test"}`)
	ts := time.Now().Unix()
	sign := sign(ts, body, secret)

	v := NewVerifier(config.SecurityConfig{
		Level:            "medium",
		Token:            "token-123",
		HMACSecret:       secret,
		TimestampSkewSec: 300,
	})
	req := httptest.NewRequest("POST", "/api/v1/notify", nil)
	req.Header.Set("Authorization", "Bearer token-123")
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Signature", sign)

	if err := v.VerifyRequest(req, body); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestVerifyStrictIPRejected(t *testing.T) {
	t.Parallel()

	secret := "hmac-secret"
	body := []byte(`{"title":"test"}`)
	ts := time.Now().Unix()
	sign := sign(ts, body, secret)

	v := NewVerifier(config.SecurityConfig{
		Level:            "strict",
		Token:            "token-123",
		HMACSecret:       secret,
		TimestampSkewSec: 300,
		IPWhitelist:      []string{"10.0.0.1"},
	})
	req := httptest.NewRequest("POST", "/api/v1/notify", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("Authorization", "Bearer token-123")
	req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-Signature", sign)

	if err := v.VerifyRequest(req, body); err == nil {
		t.Fatalf("expected strict ip error")
	}
}

func sign(ts int64, body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(ts, 10) + "." + string(body)))
	return hex.EncodeToString(mac.Sum(nil))
}
