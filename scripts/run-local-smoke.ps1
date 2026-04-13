# 本地探活：优先使用 Git Bash 执行 scripts/exec-smoke.sh（与 Linux/macOS 行为一致）。
# 若无 Git Bash，请手动安装 Git for Windows，或在仓库根执行：bash scripts/exec-smoke.sh

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$gitBash = "C:\Program Files\Git\bin\bash.exe"
if (-not (Test-Path $gitBash)) {
    Write-Error "未找到 Git Bash：$gitBash 。请安装 Git for Windows，或在 WSL 中执行 bash scripts/exec-smoke.sh"
}
Set-Location $root
& $gitBash -lc "cd '$($root -replace '\\','/')' && bash scripts/exec-smoke.sh"
exit $LASTEXITCODE
