#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
# shellcheck source=/dev/null
[[ -f .env ]] && . ./.env
set +a
if [[ -z "${DATABASE_DSN:-}" ]]; then
  : "${PG_USER:?}" "${PG_PASSWORD:?}" "${PG_HOST:?}"
  PG_PORT="${PG_PORT:-5432}"
  PG_DATABASE="${PG_DATABASE:-telegram}"
  # 密码可能含特殊字符，用 Python 做 URL 编码更稳妥（不依赖 jq）
  PY="$(command -v python3 2>/dev/null || command -v python 2>/dev/null || true)"
  if [[ -z "$PY" ]]; then
    echo "需要 python 或 python3 以安全编码 DATABASE_DSN 中的密码" >&2
    exit 1
  fi
  export DATABASE_DSN="$("$PY" -c "import urllib.parse,os; print('postgres://%s:%s@%s:%s/%s?sslmode=disable'%(os.environ['PG_USER'],urllib.parse.quote(os.environ['PG_PASSWORD'],safe=''),os.environ['PG_HOST'],os.environ.get('PG_PORT','5432'),os.environ.get('PG_DATABASE','telegram')))")"
fi
export LISTEN_ADDR="${LISTEN_ADDR:-:18080}"
export SECURITY_LEVEL="${SECURITY_LEVEL:-basic}"
export AUTH_TOKEN="${AUTH_TOKEN:-local-smoke-auth-token}"
export JWT_SECRET="${JWT_SECRET:-local-jwt-secret-key-at-least-32-chars!!}"
export BOOTSTRAP_PASSWORD="${BOOTSTRAP_PASSWORD:-LocalSmoke_Admin_1}"
export TELEGRAM_BOT_TOKEN="${TELEGRAM_BOT_TOKEN:-000000:LocalPlaceholderNotSending}"
export TELEGRAM_CHAT_ID="${TELEGRAM_CHAT_ID:--1000000000000}"

PORT="${LISTEN_ADDR#:}"
BASE="http://127.0.0.1:${PORT}"

go run ./cmd/relay &
PID=$!
for i in $(seq 1 25); do
  if curl -sf "$BASE/healthz" >/dev/null; then
    echo "OK /healthz $(curl -sS "$BASE/healthz")"
    echo "OK /metrics (first lines):"
    curl -sS "$BASE/metrics" | head -n 4
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
    exit 0
  fi
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "relay exited early"
    wait "$PID" || true
    exit 2
  fi
  sleep 2
done
echo "timeout waiting for healthz"
kill "$PID" 2>/dev/null || true
exit 2
