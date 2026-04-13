package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yclenove/telegram-relay/internal/domain"
	"github.com/yclenove/telegram-relay/internal/model"
)

// Store 负责 PostgreSQL 的连接与数据访问。
type Store struct {
	pool *pgxpool.Pool
}

// ErrDestinationNotFound 表示更新规则时引用的发送目标不存在，供 API 层映射为 400。
var ErrDestinationNotFound = errors.New("destination not found")

// ErrBotNotFound 表示更新发送目标时引用的机器人不存在。
var ErrBotNotFound = errors.New("bot not found")

// AuditLogView 为管理端展示用的审计日志结构。
type AuditLogView struct {
	ID         int64     `json:"id"`
	ActorUserID *int64   `json:"actor_user_id"`
	Action     string    `json:"action"`
	ObjectType string    `json:"object_type"`
	ObjectID   string    `json:"object_id"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pg pool failed: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping db failed: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// ApplyMigrations 启动时按文件名顺序执行 SQL 迁移。
func (s *Store) ApplyMigrations(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir failed: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		var exists bool
		err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("query migration state failed: %w", err)
		}
		if exists {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read migration file failed: %w", err)
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration tx failed: %w", err)
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s failed: %w", name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration failed: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration failed: %w", err)
		}
	}
	return nil
}

// EnsureBootstrapData 初始化默认角色、权限和管理员。
// syncBootstrapPassword 为 true 时，若引导用户已存在，则用 passwordHash 覆盖其密码哈希（与 .env 中当前口令对齐）。
func (s *Store) EnsureBootstrapData(ctx context.Context, username, passwordHash string, syncBootstrapPassword bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	roleID := int64(0)
	err = tx.QueryRow(ctx, "SELECT id FROM roles WHERE code='super_admin'").Scan(&roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, "INSERT INTO roles(code,name) VALUES('super_admin','超级管理员') RETURNING id").Scan(&roleID); err != nil {
			return err
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	perms := []string{"bot.manage", "rule.manage", "event.read", "audit.read", "system.manage", "user.manage"}
	for _, p := range perms {
		_, _ = tx.Exec(ctx, "INSERT INTO role_permissions(role_id, permission_code) VALUES($1,$2) ON CONFLICT DO NOTHING", roleID, p)
	}
	userID := int64(0)
	err = tx.QueryRow(ctx, "SELECT id FROM users WHERE username=$1", username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, "INSERT INTO users(username,password_hash) VALUES($1,$2) RETURNING id", username, passwordHash).Scan(&userID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if syncBootstrapPassword {
		if _, err = tx.Exec(ctx, "UPDATE users SET password_hash=$2, updated_at=NOW() WHERE id=$1", userID, passwordHash); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(ctx, "INSERT INTO user_roles(user_id, role_id) VALUES($1,$2) ON CONFLICT DO NOTHING", userID, roleID)
	return tx.Commit(ctx)
}

func EncryptSecret(raw string) string {
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func DecryptSecret(enc string) string {
	out, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return ""
	}
	return string(out)
}

// CreateEventAndJob 将入站请求写入 events + dispatch_jobs。
func (s *Store) CreateEventAndJob(ctx context.Context, req model.NotifyRequest, rawBody string, destinationID int64, maxAttempts int) (int64, error) {
	labels, _ := json.Marshal(req.Labels)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var eventDBID int64
	err = tx.QueryRow(ctx, `
INSERT INTO events(event_id, source, level, title, message, labels, raw_body, status)
VALUES($1,$2,$3,$4,$5,$6,$7,'pending')
ON CONFLICT(event_id) DO UPDATE SET updated_at=NOW()
RETURNING id
`, req.EventID, req.Source, req.Level, req.Title, req.Message, string(labels), rawBody).Scan(&eventDBID)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO dispatch_jobs(event_id,destination_id,status,max_attempts,next_attempt_at)
VALUES($1,$2,'pending',$3,NOW())
`, eventDBID, destinationID, maxAttempts)
	if err != nil {
		return 0, err
	}
	return eventDBID, tx.Commit(ctx)
}

// ResolveDestinationByRules 按规则优先级匹配 destination。
func (s *Store) ResolveDestinationByRules(ctx context.Context, req model.NotifyRequest) (domain.Destination, error) {
	rows, err := s.pool.Query(ctx, `
SELECT d.id, d.bot_id, d.name, d.chat_id, d.topic_id, d.parse_mode, d.is_enabled, d.created_at, d.updated_at,
       r.match_source, r.match_level
FROM routing_rules r
JOIN destinations d ON d.id = r.destination_id
WHERE r.is_enabled = TRUE AND d.is_enabled = TRUE
ORDER BY r.priority ASC, r.id ASC
`)
	if err != nil {
		return domain.Destination{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var d domain.Destination
		var matchSource, matchLevel string
		if err := rows.Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt, &matchSource, &matchLevel); err != nil {
			return domain.Destination{}, err
		}
		if (matchSource == "" || matchSource == req.Source) && (matchLevel == "" || matchLevel == req.Level) {
			return d, nil
		}
	}
	return s.GetDefaultDestination(ctx)
}

// GetDefaultDestination 获取默认机器人下的第一个启用目标。
func (s *Store) GetDefaultDestination(ctx context.Context) (domain.Destination, error) {
	var d domain.Destination
	err := s.pool.QueryRow(ctx, `
SELECT d.id, d.bot_id, d.name, d.chat_id, d.topic_id, d.parse_mode, d.is_enabled, d.created_at, d.updated_at
FROM destinations d
JOIN bots b ON b.id = d.bot_id
WHERE b.is_default=TRUE AND b.is_enabled=TRUE AND d.is_enabled=TRUE
ORDER BY d.id ASC LIMIT 1
`).Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return domain.Destination{}, err
	}
	return d, nil
}

func (s *Store) GetBotByID(ctx context.Context, id int64) (domain.Bot, error) {
	var b domain.Bot
	err := s.pool.QueryRow(ctx, `
SELECT id,name,bot_token_enc,is_enabled,is_default,remark,created_at,updated_at
FROM bots WHERE id=$1
`, id).Scan(&b.ID, &b.Name, &b.BotTokenEnc, &b.IsEnabled, &b.IsDefault, &b.Remark, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

func (s *Store) CreateBot(ctx context.Context, name, token, remark string, isDefault bool) (domain.Bot, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Bot{}, err
	}
	defer tx.Rollback(ctx)
	if isDefault {
		_, _ = tx.Exec(ctx, "UPDATE bots SET is_default=FALSE")
	}
	var b domain.Bot
	err = tx.QueryRow(ctx, `
INSERT INTO bots(name,bot_token_enc,is_enabled,is_default,remark)
VALUES($1,$2,TRUE,$3,$4)
RETURNING id,name,bot_token_enc,is_enabled,is_default,remark,created_at,updated_at
`, name, EncryptSecret(token), isDefault, remark).Scan(&b.ID, &b.Name, &b.BotTokenEnc, &b.IsEnabled, &b.IsDefault, &b.Remark, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return domain.Bot{}, err
	}
	return b, tx.Commit(ctx)
}

func (s *Store) ListBots(ctx context.Context) ([]domain.Bot, error) {
	rows, err := s.pool.Query(ctx, "SELECT id,name,bot_token_enc,is_enabled,is_default,remark,created_at,updated_at FROM bots ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Bot
	for rows.Next() {
		var b domain.Bot
		if err := rows.Scan(&b.ID, &b.Name, &b.BotTokenEnc, &b.IsEnabled, &b.IsDefault, &b.Remark, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// UpdateBotPatch 按非 nil 字段更新机器人；更换 Token 时重新加密写入。
// 将某机器人设为默认时，会在同一事务内清除其余机器人的默认标记，与 CreateBot 行为一致。
func (s *Store) UpdateBotPatch(
	ctx context.Context,
	id int64,
	name, remark *string,
	isEnabled, isDefault *bool,
	botToken *string,
) (domain.Bot, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Bot{}, err
	}
	defer tx.Rollback(ctx)
	var b domain.Bot
	err = tx.QueryRow(ctx, `
SELECT id,name,bot_token_enc,is_enabled,is_default,remark,created_at,updated_at
FROM bots WHERE id=$1
FOR UPDATE
`, id).Scan(&b.ID, &b.Name, &b.BotTokenEnc, &b.IsEnabled, &b.IsDefault, &b.Remark, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return domain.Bot{}, err
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return domain.Bot{}, fmt.Errorf("name cannot be empty")
		}
		b.Name = n
	}
	if remark != nil {
		b.Remark = *remark
	}
	if isEnabled != nil {
		b.IsEnabled = *isEnabled
	}
	if isDefault != nil && *isDefault {
		if _, err := tx.Exec(ctx, `UPDATE bots SET is_default=FALSE WHERE id<>$1`, id); err != nil {
			return domain.Bot{}, err
		}
		b.IsDefault = true
	} else if isDefault != nil {
		b.IsDefault = *isDefault
	}
	if botToken != nil && strings.TrimSpace(*botToken) != "" {
		b.BotTokenEnc = EncryptSecret(strings.TrimSpace(*botToken))
	}
	err = tx.QueryRow(ctx, `
UPDATE bots SET name=$1,remark=$2,is_enabled=$3,is_default=$4,bot_token_enc=$5,updated_at=NOW()
WHERE id=$6
RETURNING id,name,bot_token_enc,is_enabled,is_default,remark,created_at,updated_at
`, b.Name, b.Remark, b.IsEnabled, b.IsDefault, b.BotTokenEnc, id).
		Scan(&b.ID, &b.Name, &b.BotTokenEnc, &b.IsEnabled, &b.IsDefault, &b.Remark, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return domain.Bot{}, err
	}
	return b, tx.Commit(ctx)
}

// DeleteBot 删除机器人；依赖外键级联删除其目标与关联规则。
func (s *Store) DeleteBot(ctx context.Context, id int64) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM bots WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) CreateDestination(ctx context.Context, botID int64, name, chatID, parseMode string) (domain.Destination, error) {
	var d domain.Destination
	err := s.pool.QueryRow(ctx, `
INSERT INTO destinations(bot_id,name,chat_id,parse_mode,is_enabled)
VALUES($1,$2,$3,$4,TRUE)
RETURNING id,bot_id,name,chat_id,topic_id,parse_mode,is_enabled,created_at,updated_at
`, botID, name, chatID, parseMode).Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// ListDestinations 列出全部目标并带上机器人名称，供管理端下拉与表格展示。
func (s *Store) ListDestinations(ctx context.Context) ([]domain.Destination, error) {
	rows, err := s.pool.Query(ctx, `
SELECT d.id, d.bot_id, b.name, d.name, d.chat_id, d.topic_id, d.parse_mode, d.is_enabled, d.created_at, d.updated_at
FROM destinations d
JOIN bots b ON b.id = d.bot_id
ORDER BY d.id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Destination
	for rows.Next() {
		var d domain.Destination
		if err := rows.Scan(&d.ID, &d.BotID, &d.BotName, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// GetDestinationByID 按主键读取发送目标（不含机器人名称，供更新前加载）。
func (s *Store) GetDestinationByID(ctx context.Context, id int64) (domain.Destination, error) {
	var d domain.Destination
	err := s.pool.QueryRow(ctx, `
SELECT id,bot_id,name,chat_id,topic_id,parse_mode,is_enabled,created_at,updated_at
FROM destinations WHERE id=$1
`, id).Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// UpdateDestinationPatch 部分更新发送目标；若指定 bot_id 则校验机器人存在。
func (s *Store) UpdateDestinationPatch(
	ctx context.Context,
	id int64,
	botID *int64,
	name, chatID, topicID, parseMode *string,
	isEnabled *bool,
) (domain.Destination, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Destination{}, err
	}
	defer tx.Rollback(ctx)
	var d domain.Destination
	err = tx.QueryRow(ctx, `
SELECT id,bot_id,name,chat_id,topic_id,parse_mode,is_enabled,created_at,updated_at
FROM destinations WHERE id=$1
FOR UPDATE
`, id).Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return domain.Destination{}, err
	}
	if botID != nil {
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM bots WHERE id=$1`, *botID).Scan(&one); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.Destination{}, ErrBotNotFound
			}
			return domain.Destination{}, err
		}
		d.BotID = *botID
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return domain.Destination{}, fmt.Errorf("name cannot be empty")
		}
		d.Name = n
	}
	if chatID != nil {
		c := strings.TrimSpace(*chatID)
		if c == "" {
			return domain.Destination{}, fmt.Errorf("chat_id cannot be empty")
		}
		d.ChatID = c
	}
	if topicID != nil {
		d.TopicID = *topicID
	}
	if parseMode != nil {
		d.ParseMode = strings.TrimSpace(*parseMode)
		if d.ParseMode == "" {
			d.ParseMode = "HTML"
		}
	}
	if isEnabled != nil {
		d.IsEnabled = *isEnabled
	}
	err = tx.QueryRow(ctx, `
UPDATE destinations SET bot_id=$1,name=$2,chat_id=$3,topic_id=$4,parse_mode=$5,is_enabled=$6,updated_at=NOW()
WHERE id=$7
RETURNING id,bot_id,name,chat_id,topic_id,parse_mode,is_enabled,created_at,updated_at
`, d.BotID, d.Name, d.ChatID, d.TopicID, d.ParseMode, d.IsEnabled, id).
		Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return domain.Destination{}, err
	}
	return d, tx.Commit(ctx)
}

// DeleteDestination 删除发送目标；关联路由规则由外键级联删除。
func (s *Store) DeleteDestination(ctx context.Context, id int64) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM destinations WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CreateRule 创建路由规则；matchLabelsJSON 须为 JSON 对象文本（如 {}），由调用方校验。
func (s *Store) CreateRule(ctx context.Context, name string, priority int, source, level, matchLabelsJSON string, destinationID int64) (domain.RoutingRule, error) {
	var r domain.RoutingRule
	err := s.pool.QueryRow(ctx, `
INSERT INTO routing_rules(name,priority,match_source,match_level,match_labels,destination_id,is_enabled)
VALUES($1,$2,$3,$4,$5::jsonb,$6,TRUE)
RETURNING id,name,priority,match_source,match_level,match_labels::text,destination_id,is_enabled,created_at,updated_at
`, name, priority, source, level, matchLabelsJSON, destinationID).
		Scan(&r.ID, &r.Name, &r.Priority, &r.MatchSource, &r.MatchLevel, &r.MatchLabels, &r.DestinationID, &r.IsEnabled, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) ListRules(ctx context.Context) ([]domain.RoutingRule, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,name,priority,match_source,match_level,match_labels::text,destination_id,is_enabled,created_at,updated_at
FROM routing_rules ORDER BY priority ASC,id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RoutingRule
	for rows.Next() {
		var r domain.RoutingRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Priority, &r.MatchSource, &r.MatchLevel, &r.MatchLabels, &r.DestinationID, &r.IsEnabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// UpdateRulePatch 按非 nil 字段更新路由规则；若传入 destination_id 则校验目标存在。
func (s *Store) UpdateRulePatch(
	ctx context.Context,
	id int64,
	name *string,
	priority *int,
	matchSource, matchLevel *string,
	matchLabels *string,
	destinationID *int64,
	isEnabled *bool,
) (domain.RoutingRule, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.RoutingRule{}, err
	}
	defer tx.Rollback(ctx)
	var r domain.RoutingRule
	err = tx.QueryRow(ctx, `
SELECT id,name,priority,match_source,match_level,match_labels::text,destination_id,is_enabled,created_at,updated_at
FROM routing_rules WHERE id=$1
FOR UPDATE
`, id).Scan(&r.ID, &r.Name, &r.Priority, &r.MatchSource, &r.MatchLevel, &r.MatchLabels, &r.DestinationID, &r.IsEnabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return domain.RoutingRule{}, err
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return domain.RoutingRule{}, fmt.Errorf("name cannot be empty")
		}
		r.Name = n
	}
	if priority != nil {
		r.Priority = *priority
	}
	if matchSource != nil {
		r.MatchSource = *matchSource
	}
	if matchLevel != nil {
		r.MatchLevel = *matchLevel
	}
	if matchLabels != nil {
		r.MatchLabels = strings.TrimSpace(*matchLabels)
	}
	if destinationID != nil {
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM destinations WHERE id=$1`, *destinationID).Scan(&one); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.RoutingRule{}, ErrDestinationNotFound
			}
			return domain.RoutingRule{}, err
		}
		r.DestinationID = *destinationID
	}
	if isEnabled != nil {
		r.IsEnabled = *isEnabled
	}
	err = tx.QueryRow(ctx, `
UPDATE routing_rules SET name=$1,priority=$2,match_source=$3,match_level=$4,match_labels=$5::jsonb,destination_id=$6,is_enabled=$7,updated_at=NOW()
WHERE id=$8
RETURNING id,name,priority,match_source,match_level,match_labels::text,destination_id,is_enabled,created_at,updated_at
`, r.Name, r.Priority, r.MatchSource, r.MatchLevel, r.MatchLabels, r.DestinationID, r.IsEnabled, id).
		Scan(&r.ID, &r.Name, &r.Priority, &r.MatchSource, &r.MatchLevel, &r.MatchLabels, &r.DestinationID, &r.IsEnabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return domain.RoutingRule{}, err
	}
	return r, tx.Commit(ctx)
}

// DeleteRule 按主键删除单条路由规则。
func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM routing_rules WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) NextPendingJobs(ctx context.Context, limit int) ([]domain.DispatchJob, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,event_id,destination_id,status,attempt_count,max_attempts,last_error,next_attempt_at,locked_at,created_at,updated_at
FROM dispatch_jobs
WHERE status='pending' AND (next_attempt_at IS NULL OR next_attempt_at<=NOW())
ORDER BY id ASC LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DispatchJob
	for rows.Next() {
		var j domain.DispatchJob
		if err := rows.Scan(&j.ID, &j.EventID, &j.DestinationID, &j.Status, &j.AttemptCount, &j.MaxAttempts, &j.LastError, &j.NextAttemptAt, &j.LockedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (s *Store) LoadDispatchContext(ctx context.Context, jobID int64) (domain.DispatchJob, domain.Event, domain.Destination, domain.Bot, error) {
	var j domain.DispatchJob
	var e domain.Event
	var d domain.Destination
	var b domain.Bot
	err := s.pool.QueryRow(ctx, `
SELECT j.id,j.event_id,j.destination_id,j.status,j.attempt_count,j.max_attempts,j.last_error,j.next_attempt_at,j.locked_at,j.created_at,j.updated_at,
       e.id,e.event_id,e.source,e.level,e.title,e.message,e.labels::text,e.raw_body::text,e.status,e.created_at,e.updated_at,
       d.id,d.bot_id,d.name,d.chat_id,d.topic_id,d.parse_mode,d.is_enabled,d.created_at,d.updated_at,
       b.id,b.name,b.bot_token_enc,b.is_enabled,b.is_default,b.remark,b.created_at,b.updated_at
FROM dispatch_jobs j
JOIN events e ON e.id=j.event_id
JOIN destinations d ON d.id=j.destination_id
JOIN bots b ON b.id=d.bot_id
WHERE j.id=$1
`, jobID).Scan(
		&j.ID, &j.EventID, &j.DestinationID, &j.Status, &j.AttemptCount, &j.MaxAttempts, &j.LastError, &j.NextAttemptAt, &j.LockedAt, &j.CreatedAt, &j.UpdatedAt,
		&e.ID, &e.EventID, &e.Source, &e.Level, &e.Title, &e.Message, &e.Labels, &e.RawBody, &e.Status, &e.CreatedAt, &e.UpdatedAt,
		&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt,
		&b.ID, &b.Name, &b.BotTokenEnc, &b.IsEnabled, &b.IsDefault, &b.Remark, &b.CreatedAt, &b.UpdatedAt,
	)
	return j, e, d, b, err
}

func (s *Store) MarkJobSuccess(ctx context.Context, jobID int64, eventID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "UPDATE dispatch_jobs SET status='sent',updated_at=NOW() WHERE id=$1", jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE events SET status='sent',updated_at=NOW() WHERE id=$1", eventID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkJobFailedOrRetry(ctx context.Context, job domain.DispatchJob, eventID int64, lastErr string, backoff time.Duration) error {
	nextAttempt := time.Now().Add(backoff)
	status := "pending"
	eventStatus := "pending"
	attempt := job.AttemptCount + 1
	if attempt >= job.MaxAttempts {
		status = "failed"
		eventStatus = "failed"
	}
	_, err := s.pool.Exec(ctx, `
UPDATE dispatch_jobs
SET attempt_count=$2,last_error=$3,status=$4,next_attempt_at=$5,updated_at=NOW()
WHERE id=$1
`, job.ID, attempt, lastErr, status, nextAttempt)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, "UPDATE events SET status=$2,updated_at=NOW() WHERE id=$1", eventID, eventStatus)
	return err
}

func (s *Store) FindUserWithPermissions(ctx context.Context, username string) (domain.User, []string, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, "SELECT id,username,password_hash,is_enabled,created_at,updated_at FROM users WHERE username=$1", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return domain.User{}, nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT rp.permission_code
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id=ur.role_id
WHERE ur.user_id=$1
`, u.ID)
	if err != nil {
		return domain.User{}, nil, err
	}
	defer rows.Close()
	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return domain.User{}, nil, err
		}
		perms = append(perms, p)
	}
	return u, perms, nil
}

func (s *Store) WriteAudit(ctx context.Context, actorUserID *int64, action, objectType, objectID, detailJSON string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO audit_logs(actor_user_id,action,object_type,object_id,detail)
VALUES($1,$2,$3,$4,$5::jsonb)
`, actorUserID, action, objectType, objectID, detailJSON)
	return err
}

// ListEvents 按筛选与分页返回事件列表及符合条件的总数（用于管理端表格分页）。
func (s *Store) ListEvents(ctx context.Context, source, level, status string, limit, offset int) ([]domain.Event, int64, error) {
	where, args := buildEventWhere(source, level, status)
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(1) FROM events WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	lim := len(args) + 1
	off := len(args) + 2
	q := fmt.Sprintf(`
SELECT id,event_id,source,level,title,message,labels::text,raw_body::text,status,created_at,updated_at
FROM events WHERE %s ORDER BY id DESC LIMIT $%d OFFSET $%d`, where, lim, off)
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.pool.Query(ctx, q, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.EventID, &e.Source, &e.Level, &e.Title, &e.Message, &e.Labels, &e.RawBody, &e.Status, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, nil
}

// GetEventByID 按主键读取完整事件（含 message/raw_body，供管理端详情）。
func (s *Store) GetEventByID(ctx context.Context, id int64) (domain.Event, error) {
	var e domain.Event
	err := s.pool.QueryRow(ctx, `
SELECT id,event_id,source,level,title,message,labels::text,raw_body::text,status,created_at,updated_at
FROM events WHERE id=$1
`, id).Scan(&e.ID, &e.EventID, &e.Source, &e.Level, &e.Title, &e.Message, &e.Labels, &e.RawBody, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	return e, err
}

// ListDispatchJobs 分页列出异步发送任务，可按 status 精确筛选。
func (s *Store) ListDispatchJobs(ctx context.Context, status string, limit, offset int) ([]domain.DispatchJob, int64, error) {
	where, args := buildDispatchWhere(status)
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(1) FROM dispatch_jobs WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	lim := len(args) + 1
	off := len(args) + 2
	q := fmt.Sprintf(`
SELECT id,event_id,destination_id,status,attempt_count,max_attempts,last_error,next_attempt_at,locked_at,created_at,updated_at
FROM dispatch_jobs WHERE %s ORDER BY id DESC LIMIT $%d OFFSET $%d`, where, lim, off)
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.pool.Query(ctx, q, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.DispatchJob
	for rows.Next() {
		var j domain.DispatchJob
		if err := rows.Scan(&j.ID, &j.EventID, &j.DestinationID, &j.Status, &j.AttemptCount, &j.MaxAttempts, &j.LastError, &j.NextAttemptAt, &j.LockedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, nil
}

func (s *Store) DashboardStats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{
		"events_total":       0,
		"events_sent":        0,
		"events_failed":      0,
		"events_last_24h":    0,
		"jobs_pending":       0,
		"jobs_failed":           0,
		"jobs_failed_last_24h": 0,
		"bots_enabled":       0,
		"rules_enabled":      0,
	}
	queryPairs := map[string]string{
		"events_total":  "SELECT COUNT(1) FROM events",
		"events_sent":   "SELECT COUNT(1) FROM events WHERE status='sent'",
		"events_failed": "SELECT COUNT(1) FROM events WHERE status='failed'",
		"events_last_24h": `SELECT COUNT(1) FROM events WHERE created_at >= NOW() - INTERVAL '24 hours'`,
		"jobs_pending":  "SELECT COUNT(1) FROM dispatch_jobs WHERE status='pending'",
		"jobs_failed":   "SELECT COUNT(1) FROM dispatch_jobs WHERE status='failed'",
		"jobs_failed_last_24h": `SELECT COUNT(1) FROM dispatch_jobs WHERE status='failed' AND updated_at >= NOW() - INTERVAL '24 hours'`,
		"bots_enabled":  "SELECT COUNT(1) FROM bots WHERE is_enabled=TRUE",
		"rules_enabled": "SELECT COUNT(1) FROM routing_rules WHERE is_enabled=TRUE",
	}
	for k, q := range queryPairs {
		var value int64
		if err := s.pool.QueryRow(ctx, q).Scan(&value); err != nil {
			return nil, err
		}
		stats[k] = value
	}
	return stats, nil
}

// ListAuditLogs 按筛选与分页返回审计记录及总数。
func (s *Store) ListAuditLogs(
	ctx context.Context,
	action, objectType, objectID string,
	actorUserID *int64,
	createdAfter, createdBefore *time.Time,
	limit, offset int,
) ([]AuditLogView, int64, error) {
	where, args := buildAuditWhere(action, objectType, objectID, actorUserID, createdAfter, createdBefore)
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(1) FROM audit_logs WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	lim := len(args) + 1
	off := len(args) + 2
	q := fmt.Sprintf(`
SELECT id,actor_user_id,action,object_type,object_id,detail::text,created_at
FROM audit_logs WHERE %s ORDER BY id DESC LIMIT $%d OFFSET $%d`, where, lim, off)
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.pool.Query(ctx, q, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditLogView
	for rows.Next() {
		var a AuditLogView
		if err := rows.Scan(&a.ID, &a.ActorUserID, &a.Action, &a.ObjectType, &a.ObjectID, &a.Detail, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, nil
}
