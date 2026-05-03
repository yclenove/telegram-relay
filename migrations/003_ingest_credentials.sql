-- 多接入方入站凭证：独立 Bearer（key_id.secret）与独立 HMAC 密钥；事件可关联凭证便于审计。
CREATE TABLE IF NOT EXISTS ingest_credentials (
    id BIGSERIAL PRIMARY KEY,
    key_id TEXT NOT NULL UNIQUE,
    token_hash TEXT NOT NULL,
    hmac_secret_enc TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    remark TEXT NOT NULL DEFAULT '',
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE events ADD COLUMN IF NOT EXISTS ingest_credential_id BIGINT REFERENCES ingest_credentials(id) ON DELETE SET NULL;

-- 已有库的 super_admin 补 ingest_credential.manage
INSERT INTO role_permissions(role_id, permission_code)
SELECT id, 'ingest_credential.manage' FROM roles WHERE code = 'super_admin'
ON CONFLICT DO NOTHING;
