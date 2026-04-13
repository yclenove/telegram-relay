package domain

import "time"

// Bot 表示一个可用的 Telegram 机器人。
type Bot struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	BotTokenEnc string    `json:"-"`
	IsEnabled   bool      `json:"is_enabled"`
	IsDefault   bool      `json:"is_default"`
	Remark      string    `json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Destination 表示一个目标聊天配置（chat/topic/格式）。
// BotName 仅在列表接口联表查询时填充，便于管理端展示。
type Destination struct {
	ID         int64     `json:"id"`
	BotID      int64     `json:"bot_id"`
	BotName    string    `json:"bot_name,omitempty"`
	Name       string    `json:"name"`
	ChatID     string    `json:"chat_id"`
	TopicID    string    `json:"topic_id"`
	ParseMode  string    `json:"parse_mode"`
	IsEnabled  bool      `json:"is_enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RoutingRule 定义告警到目标的匹配规则。
type RoutingRule struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Priority      int       `json:"priority"`
	MatchSource   string    `json:"match_source"`
	MatchLevel    string    `json:"match_level"`
	MatchLabels   string    `json:"match_labels"`
	DestinationID int64     `json:"destination_id"`
	IsEnabled     bool      `json:"is_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Event 表示入站告警事件。
type Event struct {
	ID        int64     `json:"id"`
	EventID   string    `json:"event_id"`
	Source    string    `json:"source"`
	Level     string    `json:"level"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Labels    string    `json:"labels"`
	RawBody   string    `json:"raw_body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DispatchJob 表示异步发送任务。
type DispatchJob struct {
	ID              int64      `json:"id"`
	EventID         int64      `json:"event_id"`
	DestinationID   int64      `json:"destination_id"`
	Status          string     `json:"status"`
	AttemptCount    int        `json:"attempt_count"`
	MaxAttempts     int        `json:"max_attempts"`
	LastError       string     `json:"last_error"`
	NextAttemptAt   *time.Time `json:"next_attempt_at"`
	LockedAt        *time.Time `json:"locked_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// User 表示管理后台账号。
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsEnabled    bool      `json:"is_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Role 表示 RBAC 角色。
type Role struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserSummary 用于管理端用户列表（不含密码哈希）。
type UserSummary struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	RoleIDs   []int64   `json:"role_ids"`
}
