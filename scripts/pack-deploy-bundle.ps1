# 将 Linux amd64 relay + migrations + .env.example + 管理台静态资源 打入 release/deploy-bundle/
# 在 telegram-notification 仓库根执行：  pwsh -File scripts/pack-deploy-bundle.ps1
# 可选：指定管理台仓库路径  -AdminRepo "D:\path\to\telegram-relay-admin"
param(
    [string]$AdminRepo = "",
    [switch]$SkipAdminBuild,
    [switch]$Zip
)

$ErrorActionPreference = "Stop"
# 本文件位于 telegram-notification/scripts/
$NotificationRoot = Split-Path $PSScriptRoot -Parent
if (-not (Test-Path (Join-Path $NotificationRoot "go.mod"))) {
    Write-Error "找不到 telegram-notification 仓库根（含 go.mod）。请从 scripts 目录用 -File 调用本脚本。"
}

if (-not $AdminRepo) {
    $AdminRepo = Join-Path (Split-Path $NotificationRoot -Parent) "telegram-relay-admin"
}
if (-not (Test-Path (Join-Path $AdminRepo "package.json"))) {
    Write-Error "管理台仓库不存在: $AdminRepo （可用 -AdminRepo 指定）"
}

$Bundle = Join-Path $NotificationRoot "release\deploy-bundle"
New-Item -ItemType Directory -Force -Path $Bundle | Out-Null

Write-Host "== 1/4 构建管理台 (npm run build) ==" -ForegroundColor Cyan
if (-not $SkipAdminBuild) {
    Push-Location $AdminRepo
    try {
        if (-not (Test-Path "node_modules")) { npm install }
        npm run build
    }
    finally { Pop-Location }
}
$AdminDist = Join-Path $AdminRepo "admin-dist"
if (-not (Test-Path (Join-Path $AdminDist "index.html"))) {
    Write-Error "未找到 $AdminDist\index.html，请先 npm run build 或去掉 -SkipAdminBuild"
}

Write-Host "== 2/4 交叉编译 relay (linux/amd64) ==" -ForegroundColor Cyan
$relayOut = Join-Path $Bundle "relay"
if (Test-Path $relayOut) { Remove-Item -Force $relayOut }
Push-Location $NotificationRoot
try {
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags="-s -w" -o $relayOut .\cmd\relay
}
finally {
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    $env:CGO_ENABLED = "0"
    Pop-Location
}

Write-Host "== 3/4 同步 migrations、.env.example、admin/ ==" -ForegroundColor Cyan
$MigSrc = Join-Path $NotificationRoot "migrations"
$MigDst = Join-Path $Bundle "migrations"
if (Test-Path $MigDst) { Remove-Item -Recurse -Force $MigDst }
Copy-Item -Recurse $MigSrc $MigDst
Copy-Item -Force (Join-Path $NotificationRoot ".env.example") (Join-Path $Bundle ".env.example")

$AdminDst = Join-Path $Bundle "admin"
if (Test-Path $AdminDst) { Remove-Item -Recurse -Force $AdminDst }
New-Item -ItemType Directory -Path $AdminDst | Out-Null
Copy-Item -Recurse (Join-Path $AdminDist "*") $AdminDst

Write-Host "== 4/4 完成: $Bundle ==" -ForegroundColor Green
Get-ChildItem $Bundle | Format-Table Name, Length, LastWriteTime

if ($Zip) {
    $zipPath = Join-Path $NotificationRoot "release\telegram-relay-deploy-linux-amd64.zip"
    if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
    Compress-Archive -Path (Join-Path $Bundle "*") -DestinationPath $zipPath
    Write-Host "已生成: $zipPath" -ForegroundColor Green
}
