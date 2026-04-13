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

	"telegram-notification/internal/domain"
	"telegram-notification/internal/model"
)

// Store 负责 PostgreSQL 的连接与数据访问。
type Store struct {
	pool *pgxpool.Pool
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
func (s *Store) EnsureBootstrapData(ctx context.Context, username, passwordHash string) error {
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
	perms := []string{"bot.manage", "rule.manage", "event.read", "audit.read", "system.manage"}
	for _, p := range perms {
		_, _ = tx.Exec(ctx, "INSERT INTO role_permissions(role_id, permission_code) VALUES($1,$2) ON CONFLICT DO NOTHING", roleID, p)
	}
	userID := int64(0)
	err = tx.QueryRow(ctx, "SELECT id FROM users WHERE username=$1", username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, "INSERT INTO users(username,password_hash) VALUES($1,$2) RETURNING id", username, passwordHash).Scan(&userID); err != nil {
			return err
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
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

func (s *Store) CreateDestination(ctx context.Context, botID int64, name, chatID, parseMode string) (domain.Destination, error) {
	var d domain.Destination
	err := s.pool.QueryRow(ctx, `
INSERT INTO destinations(bot_id,name,chat_id,parse_mode,is_enabled)
VALUES($1,$2,$3,$4,TRUE)
RETURNING id,bot_id,name,chat_id,topic_id,parse_mode,is_enabled,created_at,updated_at
`, botID, name, chatID, parseMode).Scan(&d.ID, &d.BotID, &d.Name, &d.ChatID, &d.TopicID, &d.ParseMode, &d.IsEnabled, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *Store) CreateRule(ctx context.Context, name string, priority int, source, level string, destinationID int64) (domain.RoutingRule, error) {
	var r domain.RoutingRule
	err := s.pool.QueryRow(ctx, `
INSERT INTO routing_rules(name,priority,match_source,match_level,match_labels,destination_id,is_enabled)
VALUES($1,$2,$3,$4,'{}',$5,TRUE)
RETURNING id,name,priority,match_source,match_level,match_labels::text,destination_id,is_enabled,created_at,updated_at
`, name, priority, source, level, destinationID).
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

func (s *Store) ListEvents(ctx context.Context, limit int) ([]domain.Event, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,event_id,source,level,title,message,labels::text,raw_body::text,status,created_at,updated_at
FROM events ORDER BY id DESC LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.EventID, &e.Source, &e.Level, &e.Title, &e.Message, &e.Labels, &e.RawBody, &e.Status, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) DashboardStats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{
		"events_total":    0,
		"events_sent":     0,
		"events_failed":   0,
		"jobs_pending":    0,
		"bots_enabled":    0,
		"rules_enabled":   0,
	}
	queryPairs := map[string]string{
		"events_total":  "SELECT COUNT(1) FROM events",
		"events_sent":   "SELECT COUNT(1) FROM events WHERE status='sent'",
		"events_failed": "SELECT COUNT(1) FROM events WHERE status='failed'",
		"jobs_pending":  "SELECT COUNT(1) FROM dispatch_jobs WHERE status='pending'",
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
