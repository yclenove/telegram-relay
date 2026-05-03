#!/usr/bin/env bash
# 生成 AUTH_TOKEN、JWT_SECRET（openssl 随机 hex），复制到 .env。
# 用法：bash scripts/generate-env-secrets.sh

set -euo pipefail
echo "# 以下为随机生成，粘贴进 .env；勿提交 .env 到 Git"
echo "AUTH_TOKEN=$(openssl rand -hex 32)"
echo "JWT_SECRET=$(openssl rand -hex 32)"
echo "# BOOTSTRAP_PASSWORD 请自行设强口令，本脚本不生成"
