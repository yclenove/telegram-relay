package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yclenove/telegram-relay/internal/config"
)

type Verifier struct {
	level            string
	token            string
	hmacSecret       string
	timestampSkewSec int64
	ipWhitelist      map[string]struct{}
}

// NewVerifier 构建请求校验器，并将白名单转为 map 提升匹配效率。
func NewVerifier(cfg config.SecurityConfig) *Verifier {
	whitelist := make(map[string]struct{}, len(cfg.IPWhitelist))
	for _, ip := range cfg.IPWhitelist {
		whitelist[strings.TrimSpace(ip)] = struct{}{}
	}
	return &Verifier{
		level:            cfg.Level,
		token:            cfg.Token,
		hmacSecret:       cfg.HMACSecret,
		timestampSkewSec: cfg.TimestampSkewSec,
		ipWhitelist:      whitelist,
	}
}

// VerifyRequest 按安全等级逐级执行校验。
func (v *Verifier) VerifyRequest(r *http.Request, body []byte) error {
	if err := v.verifyToken(r); err != nil {
		return err
	}
	if v.level == "basic" {
		return nil
	}
	timestamp, err := v.verifyTimestamp(r)
	if err != nil {
		return err
	}
	if err := v.verifySignature(r, body, timestamp); err != nil {
		return err
	}
	if v.level == "strict" {
		return v.verifyIPWhitelist(r)
	}
	return nil
}

// verifyToken 校验 Authorization: Bearer <token>。
func (v *Verifier) verifyToken(r *http.Request) error {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if subtle.ConstantTimeCompare([]byte(token), []byte(v.token)) != 1 {
		return errors.New("invalid auth token")
	}
	return nil
}

// verifyTimestamp 校验时间戳合法性和时间窗，防止重放请求。
func (v *Verifier) verifyTimestamp(r *http.Request) (int64, error) {
	tsRaw := strings.TrimSpace(r.Header.Get("X-Timestamp"))
	if tsRaw == "" {
		return 0, errors.New("missing X-Timestamp header")
	}
	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return 0, errors.New("invalid X-Timestamp header")
	}
	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > v.timestampSkewSec {
		return 0, fmt.Errorf("timestamp exceeds skew window: %ds", v.timestampSkewSec)
	}
	return ts, nil
}

// verifySignature 校验签名：
// sign = hex(HMAC_SHA256(secret, "<timestamp>.<rawBody>"))
func (v *Verifier) verifySignature(r *http.Request, body []byte, timestamp int64) error {
	return verifySignatureWithSecret(r, body, timestamp, v.hmacSecret)
}

// verifySignatureWithSecret 供 CompositeVerifier 在命中数据库凭证时使用独立 HMAC 密钥。
func verifySignatureWithSecret(r *http.Request, body []byte, timestamp int64, hmacSecret string) error {
	signature := strings.TrimSpace(r.Header.Get("X-Signature"))
	if signature == "" {
		return errors.New("missing X-Signature header")
	}
	payload := fmt.Sprintf("%d.%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(hmacSecret))
	_, _ = mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) != 1 {
		return errors.New("invalid signature")
	}
	return nil
}

// verifyIPWhitelist 严格模式下校验来源地址是否允许。
func (v *Verifier) verifyIPWhitelist(r *http.Request) error {
	if len(v.ipWhitelist) == 0 {
		return errors.New("strict mode requires ip whitelist")
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if _, ok := v.ipWhitelist[host]; !ok {
		return fmt.Errorf("source ip not allowed: %s", host)
	}
	return nil
}
