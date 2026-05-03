package security

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/yclenove/telegram-relay/internal/config"
	"github.com/yclenove/telegram-relay/internal/service"
)

// IngestCredentialLookup 由 store 实现：按 key_id 拉取启用的凭证校验材料。
type IngestCredentialLookup interface {
	GetIngestCredentialAuthByKeyID(ctx context.Context, keyID string) (id int64, tokenHash string, hmacSecretEnc string, err error)
}

// CompositeVerifier 先校验全局 AUTH_TOKEN，再尝试数据库入站凭证（Bearer 形态 key_id.secret）。
type CompositeVerifier struct {
	cfg    config.SecurityConfig
	global *Verifier
	lookup IngestCredentialLookup
}

// NewCompositeVerifier 组装入站校验器；lookup 可为 nil（仅全局 Token）。
func NewCompositeVerifier(cfg config.SecurityConfig, lookup IngestCredentialLookup) *CompositeVerifier {
	return &CompositeVerifier{
		cfg:    cfg,
		global: NewVerifier(cfg),
		lookup: lookup,
	}
}

// VerifyRequest 校验通过后把 ingest credential id 写入返回请求的 context：非 nil 指针表示 DB 凭证 id，显式 nil 表示全局 Token。
func (c *CompositeVerifier) VerifyRequest(r *http.Request, body []byte) (*http.Request, error) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token == "" {
		return nil, errors.New("missing bearer token")
	}

	ctx := r.Context()

	if len(token) == len(c.cfg.Token) && subtle.ConstantTimeCompare([]byte(token), []byte(c.cfg.Token)) == 1 {
		if err := c.verifyAfterBearer(r, body, c.cfg.HMACSecret); err != nil {
			return nil, err
		}
		var nilID *int64
		return r.WithContext(WithIngestCredentialID(ctx, nilID)), nil
	}

	if c.lookup == nil {
		return nil, errors.New("invalid auth token")
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, errors.New("invalid auth token")
	}
	keyID := strings.TrimSpace(parts[0])
	rowID, th, hmacEnc, err := c.lookup.GetIngestCredentialAuthByKeyID(r.Context(), keyID)
	if err != nil {
		return nil, errors.New("invalid auth token")
	}
	if !service.VerifyPassword(th, token) {
		return nil, errors.New("invalid auth token")
	}
	hmacSecret := decodeSecretEnc(hmacEnc)
	if hmacSecret == "" {
		return nil, errors.New("invalid auth token")
	}
	if err := c.verifyAfterBearer(r, body, hmacSecret); err != nil {
		return nil, err
	}
	id := rowID
	return r.WithContext(WithIngestCredentialID(ctx, &id)), nil
}

func (c *CompositeVerifier) verifyAfterBearer(r *http.Request, body []byte, hmacSecret string) error {
	if c.cfg.Level == "basic" {
		return nil
	}
	ts, err := c.global.verifyTimestamp(r)
	if err != nil {
		return err
	}
	if err := verifySignatureWithSecret(r, body, ts, hmacSecret); err != nil {
		return err
	}
	if c.cfg.Level == "strict" {
		return c.global.verifyIPWhitelist(r)
	}
	return nil
}

func decodeSecretEnc(enc string) string {
	out, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return ""
	}
	return string(out)
}
