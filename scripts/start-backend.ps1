# 学习平台 后端本地启动脚本（参考 videonest 的 start-backend.ps1）
# 用法：
#   1. 先启动数据库和 Redis 容器：docker compose up -d mysql redis
#   2. 再运行本脚本：.\start-backend.ps1
# 脚本会读取项目根目录 .env，拼好 MYSQL_DSN / REDIS_HOST / REDIS_PASS 环境变量，
# 然后在本机用 go run 直接跑后端（不走 Docker，方便断点调试）。

$ErrorActionPreference = "Stop"

# 脚本位于 scripts/ 下，项目根目录是脚本所在目录的上一级
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$EnvFile = Join-Path $ProjectRoot ".env"

if (-not (Test-Path -LiteralPath $EnvFile)) {
    Write-Host "未找到 .env 文件：$EnvFile" -ForegroundColor Red
    exit 1
}

# 从 .env 读取一个键的值
function Get-EnvValue([string]$Key) {
    $Line = Get-Content -LiteralPath $EnvFile | Where-Object { $_ -match "^\s*$Key\s*=" } | Select-Object -First 1
    if ($Line) {
        return ($Line -split "=", 2)[1].Trim()
    }
    return $null
}

$DbPort = Get-EnvValue "DB_PORT"
if (-not $DbPort) { $DbPort = "3316" }
$DbName = Get-EnvValue "DB_NAME"
if (-not $DbName) { $DbName = "learningplatform" }
$DbUser = Get-EnvValue "DB_USERNAME"
if (-not $DbUser) { $DbUser = "root" }
$DbPass = Get-EnvValue "DB_PASSWORD"
$RedisPort = Get-EnvValue "REDIS_PORT"
if (-not $RedisPort) { $RedisPort = "6381" }
$RedisPass = Get-EnvValue "REDIS_PASSWORD"

if (-not $DbPass -or -not $RedisPass) {
    Write-Host ".env 缺少 DB_PASSWORD 或 REDIS_PASSWORD，请检查 .env 文件" -ForegroundColor Red
    exit 1
}

# 组装 go 后端读取的环境变量（对应 etc/learningplatform-api.yaml 里的 ${...} 占位符）
$env:MYSQL_DSN = "$DbUser`:$DbPass@tcp(127.0.0.1`:$DbPort)/$DbName?charset=utf8mb4&parseTime=true&loc=Local"
$env:REDIS_HOST = "127.0.0.1`:$RedisPort"
$env:REDIS_PASS = $RedisPass

Write-Host "MYSQL_DSN = $env:MYSQL_DSN" -ForegroundColor Cyan
Write-Host "REDIS_HOST = $env:REDIS_HOST" -ForegroundColor Cyan
Write-Host "REDIS_PASS = $env:REDIS_PASS" -ForegroundColor Cyan
Write-Host "启动后端 (go run) ..." -ForegroundColor Green

Set-Location -LiteralPath (Join-Path $ProjectRoot "backend")
go run .
