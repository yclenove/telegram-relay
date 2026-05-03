package v2

// IntegrationHints 供管理端生成第三方接入文档：对外公网根 URL 与当前入站安全级别（不含密钥）。
type IntegrationHints struct {
	PublicBaseURL string
	SecurityLevel string
}
