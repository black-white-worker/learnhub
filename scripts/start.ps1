# 学习平台 一键启动脚本（参考 S:\videonest\scripts 的风格）
# 用法：
#   .\start.ps1              # 构建并启动全部服务（首次或代码改动后推荐）
#   .\start.ps1 -SkipBuild   # 不重新构建镜像，直接启动（更快）
#   .\start.ps1 -NoBrowser   # 启动后不自动打开浏览器

param(
    [switch]$SkipBuild,
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"

# 脚本位于 scripts/ 下，项目根目录是脚本所在目录的上一级
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$EnvFile = Join-Path $ProjectRoot ".env"

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  学习平台 一键启动" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 从 .env 读取一个键的值（取第一个匹配行，去掉行内注释）
function Get-EnvValue([string]$Key) {
    $Line = Get-Content -LiteralPath $EnvFile | Where-Object { $_ -match "^\s*$Key\s*=" } | Select-Object -First 1
    if ($Line) {
        $Value = ($Line -split "=", 2)[1].Trim()
        return $Value
    }
    return $null
}

# 1. 检查 .env 是否存在
if (-not (Test-Path -LiteralPath $EnvFile)) {
    Write-Host "未找到 .env 文件：$EnvFile（请确认项目文件完整）" -ForegroundColor Red
    exit 1
}

# 2. 读取对外端口（默认值兜底）
$HttpPort = Get-EnvValue "HTTP_PORT"
if (-not $HttpPort) { $HttpPort = "8088" }
$BackendPort = Get-EnvValue "BACKEND_PORT"
if (-not $BackendPort) { $BackendPort = "8888" }
$DbPort = Get-EnvValue "DB_PORT"
if (-not $DbPort) { $DbPort = "3316" }
$RedisPort = Get-EnvValue "REDIS_PORT"
if (-not $RedisPort) { $RedisPort = "6381" }

# 3. 检查 Docker 引擎是否可用；不可用时自动拉起 Docker Desktop 并等待
$dockerOk = $false
try {
    docker info 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) { $dockerOk = $true }
} catch { }

if (-not $dockerOk) {
    Write-Host "Docker 引擎未运行，正在启动 Docker Desktop ..." -ForegroundColor Yellow
    $dockerDesktop = @(
        "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe",
        "$env:LOCALAPPDATA\Programs\DockerDesktop\Docker Desktop.exe",
        "$env:LOCALAPPDATA\Docker\Docker Desktop.exe"
    ) | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (-not $dockerDesktop) {
        Write-Host "找不到 Docker Desktop，请先安装并启动 Docker，再运行本脚本" -ForegroundColor Red
        exit 1
    }
    Start-Process -FilePath $dockerDesktop

    Write-Host "等待 Docker 引擎就绪（最多 120 秒）..." -ForegroundColor Yellow
    $ready = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Seconds 3
        try {
            docker info 2>&1 | Out-Null
            if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        } catch { }
    }
    if (-not $ready) {
        Write-Host "Docker 引擎 120 秒内未就绪，请手动检查 Docker Desktop" -ForegroundColor Red
        exit 1
    }
    Write-Host "Docker 引擎已就绪。" -ForegroundColor Green
}

# 4. 构建并启动容器
Set-Location -LiteralPath $ProjectRoot
if ($SkipBuild) {
    Write-Host "跳过构建，直接启动容器 ..." -ForegroundColor Cyan
    docker compose up -d
} else {
    Write-Host "正在构建并启动容器（首次或代码改动后会自动重新构建）..." -ForegroundColor Cyan
    docker compose up -d --build
}
if ($LASTEXITCODE -ne 0) {
    Write-Host "docker compose 启动失败，请查看上面的报错" -ForegroundColor Red
    exit 1
}

# 5. 等待后端接口就绪（最多 90 秒）
Write-Host "等待服务就绪 ..." -ForegroundColor Yellow
$ready = $false
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 3
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/api/from/you" -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -eq 200) { $ready = $true; break }
    } catch { }
}
if (-not $ready) {
    Write-Host "服务未就绪，打印容器状态与后端日志：" -ForegroundColor Red
    docker compose ps
    docker compose logs --tail=60 backend
    exit 1
}

# 6. 打印访问信息
Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  启动成功！" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "  前端页面:  http://localhost:$HttpPort" -ForegroundColor Green
Write-Host "  后端 API:  http://localhost:$BackendPort"
Write-Host "  测试账号:  admin / 123456"
Write-Host "  MySQL:     127.0.0.1:$DbPort  (root / 密码见 .env)"
Write-Host "  Redis:     127.0.0.1:$RedisPort"
Write-Host "  常用命令:  docker compose logs -f backend（看后端日志）"
Write-Host "             docker compose down（停止服务）"
Write-Host "========================================" -ForegroundColor Green

# 7. 打开浏览器
if (-not $NoBrowser) {
    Start-Process "http://localhost:$HttpPort"
}
