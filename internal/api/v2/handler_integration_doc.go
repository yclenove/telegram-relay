package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// integrationHints 返回配置中的公网根地址与安全级别，供管理台预填接入文档（不含密钥）。
func (h *Handler) integrationHints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"public_base_url":     strings.TrimSuffix(strings.TrimSpace(h.hints.PublicBaseURL), "/"),
		"security_level":      strings.TrimSpace(h.hints.SecurityLevel),
		"default_notify_path": "/api/v2/notify",
	})
}

type integrationDocRequest struct {
	BaseURL          string `json:"base_url"`
	NotifyPath       string `json:"notify_path"`
	KeyID            string `json:"key_id"`
	PlainToken       string `json:"plain_token"`
	PlainHMACSecret  string `json:"plain_hmac_secret"`
	SecurityLevel    string `json:"security_level"`
	ExpiresAtRFC3339 string `json:"expires_at"`
	// DefaultSource 写入各语言示例 JSON 的 source 字段；省略且提供 key_id 时服务端会用 ingest-<key_id> 作为文档默认值。
	DefaultSource string `json:"default_source"`
}

// renderIntegrationDoc POST：根据当前密钥与公网地址生成 Markdown 接入说明（curl / Python 等），供下载。
func (h *Handler) renderIntegrationDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req integrationDocRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	base := strings.TrimSpace(req.BaseURL)
	if base == "" {
		base = strings.TrimSuffix(strings.TrimSpace(h.hints.PublicBaseURL), "/")
	}
	if base == "" {
		http.Error(w, "base_url is required (set PUBLIC_BASE_URL on server or pass base_url in body)", http.StatusBadRequest)
		return
	}
	path := strings.TrimSpace(req.NotifyPath)
	if path == "" {
		path = "/api/v2/notify"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	sec := strings.TrimSpace(req.SecurityLevel)
	if sec == "" {
		sec = strings.TrimSpace(h.hints.SecurityLevel)
	}
	if sec == "" {
		sec = "basic"
	}
	if req.PlainToken == "" || req.PlainHMACSecret == "" {
		http.Error(w, "plain_token and plain_hmac_secret are required", http.StatusBadRequest)
		return
	}
	ds := strings.TrimSpace(req.DefaultSource)
	if ds == "" && strings.TrimSpace(req.KeyID) != "" {
		ds = "ingest-" + strings.TrimSpace(req.KeyID)
	}
	notifyURL := strings.TrimSuffix(base, "/") + path
	fname := "third-party-ingest"
	if strings.TrimSpace(req.KeyID) != "" {
		fname = fmt.Sprintf("third-party-ingest-%s", strings.TrimSpace(req.KeyID))
	}
	md := buildIntegrationMarkdown(integrationDocParams{
		NotifyURL:        notifyURL,
		PlainToken:       req.PlainToken,
		PlainHMACSecret:  req.PlainHMACSecret,
		SecurityLevel:    sec,
		ExpiresAtDisplay: strings.TrimSpace(req.ExpiresAtRFC3339),
		DefaultSource:    ds,
	})
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, fname))
	_, _ = w.Write([]byte(md))
}

type integrationDocParams struct {
	NotifyURL        string
	PlainToken       string
	PlainHMACSecret  string
	SecurityLevel    string
	ExpiresAtDisplay string
	DefaultSource    string
}

func buildIntegrationMarkdown(p integrationDocParams) string {
	expLine := "本凭证 **未设置过期时间**（长期有效），除非在管理台禁用或轮换。"
	if p.ExpiresAtDisplay != "" {
		if _, err := time.Parse(time.RFC3339, p.ExpiresAtDisplay); err == nil {
			expLine = "凭证过期时间（UTC/RFC3339）：**" + p.ExpiresAtDisplay + "**。过期后该凭证将拒绝入站，请在此前轮换。"
		}
	}
	exSrc := strings.TrimSpace(p.DefaultSource)
	if exSrc == "" {
		exSrc = "your-service-name"
	}
	demo := struct {
		Title   string `json:"title"`
		Message string `json:"message"`
		Source  string `json:"source"`
		Level   string `json:"level"`
	}{Title: "测试", Message: "正文", Source: exSrc, Level: "info"}
	demoBytes, _ := json.Marshal(demo)
	demoJSON := string(demoBytes)
	demoPretty, _ := json.MarshalIndent(demo, "", "  ")
	prettyStr := string(demoPretty)

	tokQ := strconv.Quote(p.PlainToken)
	hmacQ := strconv.Quote(p.PlainHMACSecret)
	notifyQ := strconv.Quote(p.NotifyURL)
	authHQ := strconv.Quote("Authorization: Bearer " + p.PlainToken)
	demoJSONQ := strconv.Quote(demoJSON)

	var b strings.Builder
	fmt.Fprintf(&b, `# 第三方入站接入说明（自动生成）

> 入队地址：**%s**  
> 安全级别（与 relay 部署一致）：**%s**  
> %s

## 1. 鉴权说明

- 请求头：` + "`Authorization: Bearer <plain_token>`" + `  
- **plain_token** 即创建/轮换凭证时展示的整串（格式为 `+"`key_id.secret`"+`），请勿拆成两段分别传。  
- 若部署为 **medium** 或 **strict**，还需 `+"`X-Timestamp`"+`（Unix 秒）与 `+"`X-Signature`"+`（见下文）。  
- **strict** 另可能校验客户端 IP，经反代时请确认 Nginx 已传 `+"`X-Forwarded-For`"+` / `+"`X-Real-IP`"+`。

## 2. 请求体（JSON）

至少包含 **title**、**message**；**source** 建议填写（便于路由 **match_source** 命中）。使用**本入站凭证**调用 ` + "`/api/v2/notify`" + ` 时，若 JSON 中省略 **source** 字段，relay 会用凭证在管理台的**名称**作为来源；若名称为空则用 **ingest-** 前缀加上 **key_id**。

v2 可省略 **event_id** 由服务生成。

`+"```json"+`
%s`+"\n```"+`

## 3. 你的凭证占位（请妥善保管，勿提交到仓库）

`+"```"+`
PLAIN_TOKEN=%s
PLAIN_HMAC_SECRET=%s
`+"```"+`

## 4. curl 示例（basic）

`+"```bash"+`
curl -sS -X POST %s `+"\\\n"+`  -H %s `+"\\\n"+`  -H "Content-Type: application/json" `+"\\\n"+`  -d %s
`+"```"+`

## 5. Python 3 示例（basic）

`+"```python"+`
import json
import urllib.request

url = %s
body = json.loads(%s)
req = urllib.request.Request(
    url,
    data=json.dumps(body, separators=(",", ":")).encode("utf-8"),
    headers={
        "Authorization": "Bearer " + %s,
        "Content-Type": "application/json",
    },
    method="POST",
)
with urllib.request.urlopen(req, timeout=10) as resp:
    print(resp.read().decode())
`+"```"+`

## 6. Node.js 示例（basic，Node 18+ 内置 fetch）

`+"```javascript"+`
// Node 18+：node ingest-demo.js
(async () => {
  const url = %s;
  const token = %s;
  const body = %s;
  const res = await fetch(url, {
    method: "POST",
    headers: {
      Authorization: "Bearer " + token,
      "Content-Type": "application/json",
    },
    body,
  });
  console.log(await res.text());
})();
`+"```"+`

## 7. Go 示例（basic）

`+"```go"+`
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

func main() {
	url := %s
	token := %s
	body := []byte(%s)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Println(string(out))
}
`+"```"+`

## 8. PHP 示例（basic，需开启 allow_url_fopen）

`+"```php"+`
<?php
// php ingest-demo.php
$url = %s;
$token = %s;
$body = %s;
$opts = [
    'http' => [
        'method' => 'POST',
        'header' => "Authorization: Bearer {$token}\r\nContent-Type: application/json\r\n",
        'content' => $body,
        'timeout' => 10,
    ],
];
$ctx = stream_context_create($opts);
$res = file_get_contents($url, false, $ctx);
if ($res === false) {
    fwrite(STDERR, "request failed\n");
    exit(1);
}
echo $res;
`+"```"+`

`, p.NotifyURL, p.SecurityLevel, expLine, prettyStr, p.PlainToken, p.PlainHMACSecret, notifyQ, authHQ, demoJSONQ, notifyQ, demoJSONQ, tokQ, notifyQ, tokQ, demoJSONQ, notifyQ, tokQ, demoJSONQ, notifyQ, tokQ, demoJSONQ)

	if strings.EqualFold(p.SecurityLevel, "medium") || strings.EqualFold(p.SecurityLevel, "strict") {
		fmt.Fprintf(&b, `
## 9. medium / strict：签名与 Python 示例

签名字符串（UTF-8 字节与请求体**完全一致**）：

`+"```text"+`
payload = "<timestamp>." + <原始 JSON 字符串>
X-Signature = hex( HMAC_SHA256( HMAC密钥字节, payload ) )
`+"```"+`

HMAC 密钥为 **plain_hmac_secret** 的 UTF-8 字节（以下为可直接运行的 Python 片段，`+"`plain_token`"+` / 密钥已用 repr 转义）：

`+"```python"+`
import hashlib
import hmac
import json
import time
import urllib.request

url = %s
hmac_secret = %s.encode("utf-8")
raw_body = json.dumps(json.loads(%s), separators=(",", ":"))
ts = str(int(time.time()))
signing = f"{ts}.{raw_body}".encode("utf-8")
sig = hmac.new(hmac_secret, signing, hashlib.sha256).hexdigest()

req = urllib.request.Request(
    url,
    data=raw_body.encode("utf-8"),
    headers={
        "Authorization": "Bearer " + %s,
        "Content-Type": "application/json",
        "X-Timestamp": ts,
        "X-Signature": sig,
    },
    method="POST",
)
with urllib.request.urlopen(req, timeout=10) as resp:
    print(resp.read().decode())
`+"```"+`

`, strconv.Quote(p.NotifyURL), hmacQ, demoJSONQ, tokQ)
	}

	fmt.Fprintf(&b, `
## 10. 更多说明

详见仓库文档 **docs/third-party-integration.md**（与当前 relay 版本一致即可）。

---
*文档由 relay 管理 API 根据当前配置生成于 %s*
`, time.Now().Format(time.RFC3339))
	return b.String()
}
