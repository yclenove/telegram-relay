-- 入站凭证可选过期时间：NULL 表示永不过期；过期后 CompositeVerifier 查库即不匹配。
ALTER TABLE ingest_credentials ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;
