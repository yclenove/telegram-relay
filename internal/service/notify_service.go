package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yclenove/telegram-relay/internal/model"
	"github.com/yclenove/telegram-relay/internal/repository/postgres"
)

// ErrNoDestination 表示既无规则命中、也没有可用的默认机器人发送目标；管理台 notify-test 会据此返回可读错误。
var ErrNoDestination = errors.New("no destination: add a matching routing rule or set a default bot with an enabled destination")
// NotifyService 负责把入站告警转换为事件+任务。
type NotifyService struct {
	store       *postgres.Store
	maxAttempts int
}

func NewNotifyService(store *postgres.Store, maxAttempts int) *NotifyService {
	return &NotifyService{store: store, maxAttempts: maxAttempts}
}

func (s *NotifyService) Ingest(ctx context.Context, req model.NotifyRequest, rawBody []byte, ingestCredentialID *int64) (int64, error) {
	// 当上游没给 event_id 时，本地兜底生成幂等键。
	if req.EventID == "" {
		req.EventID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	// 使用数据库入站凭证且未传 source 时：用凭证名称，否则 ingest-<key_id>；并回写 rawBody 以便落库与签名一致。
	if strings.TrimSpace(req.Source) == "" && ingestCredentialID != nil {
		row, err := s.store.GetIngestCredentialByID(ctx, *ingestCredentialID)
		if err != nil {
			return 0, err
		}
		if n := strings.TrimSpace(row.Name); n != "" {
			req.Source = n
		} else {
			req.Source = "ingest-" + row.KeyID
		}
		b, err := json.Marshal(req)
		if err != nil {
			return 0, fmt.Errorf("marshal notify body: %w", err)
		}
		rawBody = b
	}
	destination, err := s.store.ResolveDestinationByRules(ctx, req)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNoDestination
		}
		return 0, err
	}
	return s.store.CreateEventAndJob(ctx, req, string(rawBody), destination.ID, s.maxAttempts, ingestCredentialID)
}

func BuildAuditDetail(data any) string {
	raw, _ := json.Marshal(data)
	return string(raw)
}
