package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// IngestCredentialSummary 管理端列表展示（不含密钥与哈希）。
type IngestCredentialSummary struct {
	ID        int64      `json:"id"`
	KeyID     string     `json:"key_id"`
	Name      string     `json:"name"`
	Remark    string     `json:"remark"`
	IsEnabled bool       `json:"is_enabled"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type ingestRowScanner interface {
	Scan(dest ...any) error
}

func scanIngestSummary(row ingestRowScanner) (IngestCredentialSummary, error) {
	var r IngestCredentialSummary
	var exp pgtype.Timestamptz
	if err := row.Scan(&r.ID, &r.KeyID, &r.Name, &r.Remark, &r.IsEnabled, &exp, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return IngestCredentialSummary{}, err
	}
	if exp.Valid {
		t := exp.Time
		r.ExpiresAt = &t
	}
	return r, nil
}

// ListIngestCredentials 列出全部入站凭证（管理端）。
func (s *Store) ListIngestCredentials(ctx context.Context) ([]IngestCredentialSummary, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, key_id, name, remark, is_enabled, expires_at, created_at, updated_at
FROM ingest_credentials
ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IngestCredentialSummary
	for rows.Next() {
		r, err := scanIngestSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetIngestCredentialAuthByKeyID 按 key_id 取启用的凭证（security.CompositeVerifier 的 IngestCredentialLookup）。
func (s *Store) GetIngestCredentialAuthByKeyID(ctx context.Context, keyID string) (id int64, tokenHash string, hmacSecretEnc string, err error) {
	err = s.pool.QueryRow(ctx, `
SELECT id, token_hash, hmac_secret_enc
FROM ingest_credentials
WHERE key_id=$1 AND is_enabled=TRUE
  AND (expires_at IS NULL OR expires_at > NOW())`, keyID).Scan(&id, &tokenHash, &hmacSecretEnc)
	return id, tokenHash, hmacSecretEnc, err
}

// CreateIngestCredential 插入一行；token_hash、hmac_secret_enc 由调用方生成；expiresAt 为 nil 表示永不过期。
func (s *Store) CreateIngestCredential(ctx context.Context, keyID, tokenHash, hmacSecretEnc, name, remark string, expiresAt *time.Time) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
INSERT INTO ingest_credentials(key_id, token_hash, hmac_secret_enc, name, remark, expires_at)
VALUES($1,$2,$3,$4,$5,$6)
RETURNING id`, keyID, tokenHash, hmacSecretEnc, name, remark, expiresAt).Scan(&id)
	return id, err
}

// UpdateIngestCredentialPatch 部分更新名称、备注、启用、过期时间。
// expiresAtSet 为 false 时不改 expires_at；为 true 时 expiresAt 为 nil 则置为 NULL（永不过期），非 nil 则写入该时刻。
func (s *Store) UpdateIngestCredentialPatch(ctx context.Context, id int64, name, remark *string, isEnabled *bool, expiresAtSet bool, expiresAt *time.Time) (IngestCredentialSummary, error) {
	row := s.pool.QueryRow(ctx, `
UPDATE ingest_credentials SET
  name = COALESCE($2, name),
  remark = COALESCE($3, remark),
  is_enabled = COALESCE($4, is_enabled),
  expires_at = CASE WHEN $5 THEN $6 ELSE expires_at END,
  updated_at = NOW()
WHERE id=$1
RETURNING id, key_id, name, remark, is_enabled, expires_at, created_at, updated_at`,
		id, name, remark, isEnabled, expiresAtSet, expiresAt)
	out, err := scanIngestSummary(row)
	if err != nil {
		return IngestCredentialSummary{}, err
	}
	return out, nil
}

// RotateIngestCredentialSecrets 轮换 token 与 HMAC 材料（哈希与 enc 由调用方计算）。
func (s *Store) RotateIngestCredentialSecrets(ctx context.Context, id int64, tokenHash, hmacSecretEnc string) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE ingest_credentials SET token_hash=$2, hmac_secret_enc=$3, updated_at=NOW() WHERE id=$1`, id, tokenHash, hmacSecretEnc)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetIngestCredentialByID 管理端按 id 取摘要（不含密文）。
func (s *Store) GetIngestCredentialByID(ctx context.Context, id int64) (IngestCredentialSummary, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, key_id, name, remark, is_enabled, expires_at, created_at, updated_at
FROM ingest_credentials WHERE id=$1`, id)
	return scanIngestSummary(row)
}
