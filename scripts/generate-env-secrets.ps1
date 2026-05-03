# 生成 AUTH_TOKEN、JWT_SECRET 的随机十六进制串（各 32 字节 → 64 个 hex 字符），复制到 .env。
# 用法：在仓库根目录执行：pwsh -File scripts/generate-env-secrets.ps1

function Get-RandomHex([int] $byteCount) {
  $bytes = New-Object byte[] $byteCount
  $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  $rng.GetBytes($bytes)
  -join ($bytes | ForEach-Object { $_.ToString('x2') })
}

Write-Host "# Paste into .env; do not commit .env" -ForegroundColor DarkGray
Write-Host ("AUTH_TOKEN=" + (Get-RandomHex 32))
Write-Host ("JWT_SECRET=" + (Get-RandomHex 32))
Write-Host "# Set BOOTSTRAP_PASSWORD yourself (strong passphrase)" -ForegroundColor DarkGray
