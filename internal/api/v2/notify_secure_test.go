package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"

	"github.com/yclenove/telegram-relay/internal/config"
	"github.com/yclenove/telegram-relay/internal/model"
	"github.com/yclenove/telegram-relay/internal/security"
	"github.com/yclenove/telegram-relay/internal/service"
)

type stubIngester struct {
	called bool
	id     int64
	err    error
}

func (s *stubIngester) Ingest(_ context.Context, _ model.NotifyRequest, _ []byte) (int64, error) {
	s.called = true
	return s.id, s.err
}

func TestNotifyV2_UnauthorizedWithoutToken(t *testing.T) {
	t.Parallel()
	stub := &stubIngester{id: 42}
	v := security.NewVerifier(config.SecurityConfig{
		Level:            "basic",
		Token:            "secret-token",
		TimestampSkewSec: 300,
	})
	h := NewHandler(slog.Default(), nil, nil, stub, v, rate.NewLimiter(100, 100))

	srv := httptest.NewServer(http.HandlerFunc(h.notifyV2Secure))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader([]byte(`{"title":"a","source":"s","message":"m"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if stub.called {
		t.Fatal("ingest should not run without auth")
	}
}

func TestNotifyV2_OKWithBearer(t *testing.T) {
	t.Parallel()
	stub := &stubIngester{id: 99}
	v := security.NewVerifier(config.SecurityConfig{
		Level:            "basic",
		Token:            "secret-token",
		TimestampSkewSec: 300,
	})
	h := NewHandler(slog.Default(), nil, nil, stub, v, rate.NewLimiter(100, 100))

	body := map[string]string{"title": "t", "source": "s", "message": "m"}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/notify", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.notifyV2Secure(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !stub.called {
		t.Fatal("expected ingest called")
	}
}

func testAccessJWT(t *testing.T, secret string, uid int64, perms []string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ":   "access",
		"uid":   float64(uid),
		"perms": perms,
		"exp":   time.Now().Add(time.Minute).Unix(),
	})
	raw, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func TestNotifyTest_UnauthorizedWithoutBearer(t *testing.T) {
	t.Parallel()
	stub := &stubIngester{id: 1}
	authSvc := service.NewAuthService(nil, config.AuthConfig{JWTSecret: "unit-secret", AccessTokenTTLMin: 60, RefreshTokenTTLMin: 1440})
	h := NewHandler(slog.Default(), nil, authSvc, stub, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/notify-test", bytes.NewReader([]byte(`{"title":"a","source":"s","message":"m"}`)))
	h.withAuth("bot.manage", http.HandlerFunc(h.notifyTest)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if stub.called {
		t.Fatal("ingest should not run")
	}
}

func TestNotifyTest_ForbiddenWithoutBotManage(t *testing.T) {
	t.Parallel()
	stub := &stubIngester{id: 2}
	authSvc := service.NewAuthService(nil, config.AuthConfig{JWTSecret: "unit-secret", AccessTokenTTLMin: 60, RefreshTokenTTLMin: 1440})
	h := NewHandler(slog.Default(), nil, authSvc, stub, nil, nil)
	raw := testAccessJWT(t, "unit-secret", 9, []string{"event.read"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/notify-test", bytes.NewReader([]byte(`{"title":"a","source":"s","message":"m"}`)))
	req.Header.Set("Authorization", "Bearer "+raw)
	h.withAuth("bot.manage", http.HandlerFunc(h.notifyTest)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if stub.called {
		t.Fatal("ingest should not run")
	}
}

func TestNotifyTest_OKWithBotManageJWT(t *testing.T) {
	t.Parallel()
	stub := &stubIngester{id: 42}
	authSvc := service.NewAuthService(nil, config.AuthConfig{JWTSecret: "unit-secret", AccessTokenTTLMin: 60, RefreshTokenTTLMin: 1440})
	h := NewHandler(slog.Default(), nil, authSvc, stub, nil, nil)
	raw := testAccessJWT(t, "unit-secret", 3, []string{"bot.manage"})
	body := []byte(`{"title":"t","source":"s","message":"m"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/notify-test", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+raw)
	h.withAuth("bot.manage", http.HandlerFunc(h.notifyTest)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !stub.called {
		t.Fatal("expected ingest called")
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "queued" {
		t.Fatalf("unexpected status: %v", resp["status"])
	}
}
