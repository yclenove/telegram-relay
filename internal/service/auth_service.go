package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/yclenove/telegram-relay/internal/config"
	"github.com/yclenove/telegram-relay/internal/repository/postgres"
)

const jwtTypAccess = "access"
const jwtTypRefresh = "refresh"

// ErrInvalidCredentials 表示用户名或口令不匹配（对外可统一文案）。
var ErrInvalidCredentials = errors.New("invalid username or password")

// AuthService 负责后台账号认证与 JWT 签发。
type AuthService struct {
	store *postgres.Store
	cfg   config.AuthConfig
}

func NewAuthService(store *postgres.Store, cfg config.AuthConfig) *AuthService {
	return &AuthService{store: store, cfg: cfg}
}

func isLegacySHA256Hex(stored string) bool {
	if len(stored) != 64 {
		return false
	}
	for _, c := range stored {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func hashSHA256Legacy(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// HashPassword 使用 bcrypt 生成新口令哈希（新建用户、bootstrap 等）。
func HashPassword(raw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword 同时支持 bcrypt 与历史 SHA256(hex) 口令，便于无停机迁移。
func VerifyPassword(storedHash, rawPassword string) bool {
	if strings.HasPrefix(storedHash, "$2a$") || strings.HasPrefix(storedHash, "$2b$") || strings.HasPrefix(storedHash, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(rawPassword)) == nil
	}
	if isLegacySHA256Hex(storedHash) {
		return subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashSHA256Legacy(rawPassword))) == 1
	}
	return false
}

func jwtParserOpts() []jwt.ParserOption {
	return []jwt.ParserOption{jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})}
}

func (s *AuthService) issueAccessAndRefresh(userID int64, username string, permList []string) (access, refresh string, err error) {
	if permList == nil {
		permList = []string{}
	}
	now := time.Now()
	accessTok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ":   jwtTypAccess,
		"uid":   userID,
		"uname": username,
		"perms": permList,
		"exp":   now.Add(time.Duration(s.cfg.AccessTokenTTLMin) * time.Minute).Unix(),
		"iat":   now.Unix(),
	})
	access, err = accessTok.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", "", err
	}
	refreshTok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ": jwtTypRefresh,
		"uid": userID,
		"exp": now.Add(time.Duration(s.cfg.RefreshTokenTTLMin) * time.Minute).Unix(),
		"iat": now.Unix(),
	})
	refresh, err = refreshTok.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// Login 校验账号口令并签发 access / refresh JWT。
func (s *AuthService) Login(ctx context.Context, username, password string) (accessToken, refreshToken string, perms []string, uid int64, err error) {
	user, permList, err := s.store.FindUserWithPermissions(ctx, username)
	if err != nil {
		return "", "", nil, 0, err
	}
	if !user.IsEnabled {
		return "", "", nil, 0, errors.New("user disabled")
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return "", "", nil, 0, ErrInvalidCredentials
	}
	// 登录成功后把历史 SHA256 哈希升级为 bcrypt，后续校验成本更符合常见后台实践。
	if isLegacySHA256Hex(user.PasswordHash) {
		newHash, hErr := HashPassword(password)
		if hErr == nil {
			if upErr := s.store.UpdateUserPasswordHash(ctx, user.ID, newHash); upErr != nil {
				// 不影响本次登录；下次仍可用旧哈希验证。
			}
		}
	}
	accessToken, refreshToken, err = s.issueAccessAndRefresh(user.ID, user.Username, permList)
	if err != nil {
		return "", "", nil, 0, err
	}
	if permList == nil {
		permList = []string{}
	}
	return accessToken, refreshToken, permList, user.ID, nil
}

// Refresh 校验 refresh JWT 并签发新的 access + refresh（旋转 refresh，降低被盗窗口）。
func (s *AuthService) Refresh(ctx context.Context, refreshRaw string) (accessToken, refreshToken string, perms []string, err error) {
	raw := strings.TrimSpace(refreshRaw)
	if raw == "" {
		return "", "", nil, errors.New("missing refresh token")
	}
	tok, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %T", token.Method)
		}
		return []byte(s.cfg.JWTSecret), nil
	}, jwtParserOpts()...)
	if err != nil || !tok.Valid {
		return "", "", nil, errors.New("invalid refresh token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", nil, errors.New("invalid refresh token")
	}
	if typ, _ := claims["typ"].(string); typ != jwtTypRefresh {
		return "", "", nil, errors.New("invalid refresh token")
	}
	uidFloat, ok := claims["uid"].(float64)
	if !ok {
		return "", "", nil, errors.New("invalid refresh token")
	}
	user, permList, err := s.store.FindUserWithPermissionsByID(ctx, int64(uidFloat))
	if err != nil || !user.IsEnabled {
		return "", "", nil, errors.New("invalid refresh token")
	}
	accessToken, refreshToken, err = s.issueAccessAndRefresh(user.ID, user.Username, permList)
	if err != nil {
		return "", "", nil, err
	}
	if permList == nil {
		permList = []string{}
	}
	return accessToken, refreshToken, permList, nil
}

// ParseToken 解析管理端 access JWT，返回用户 id 与权限集合。
// 拒绝 refresh 类型 JWT，避免误把长期令牌当会话使用。
func (s *AuthService) ParseToken(raw string) (int64, map[string]bool, error) {
	tok, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %T", token.Method)
		}
		return []byte(s.cfg.JWTSecret), nil
	}, jwtParserOpts()...)
	if err != nil || !tok.Valid {
		return 0, nil, errors.New("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, nil, errors.New("invalid claims")
	}
	if typ, ok := claims["typ"].(string); ok && typ != jwtTypAccess {
		return 0, nil, errors.New("invalid token type")
	}
	uidFloat, ok := claims["uid"].(float64)
	if !ok {
		return 0, nil, errors.New("invalid uid")
	}
	perms := map[string]bool{}
	if arr, ok := claims["perms"].([]interface{}); ok {
		for _, v := range arr {
			if ps, ok := v.(string); ok {
				perms[ps] = true
			}
		}
	}
	return int64(uidFloat), perms, nil
}
