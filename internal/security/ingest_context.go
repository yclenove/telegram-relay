package security

import "context"

// ingestCredCtxKey 用于在 notify 成功后把「哪条入站凭证通过校验」写入 context，供入队写库关联。
type ingestCredCtxKey struct{}

// WithIngestCredentialID 将匹配到的 ingest_credentials.id 写入 context（nil 表示走全局 AUTH_TOKEN）。
func WithIngestCredentialID(ctx context.Context, id *int64) context.Context {
	return context.WithValue(ctx, ingestCredCtxKey{}, id)
}

// IngestCredentialIDFromContext 读取入站凭证 id；第二个返回值表示是否曾设置过该键（含显式 nil）。
func IngestCredentialIDFromContext(ctx context.Context) (*int64, bool) {
	v, ok := ctx.Value(ingestCredCtxKey{}).(*int64)
	if !ok {
		return nil, false
	}
	return v, true
}
