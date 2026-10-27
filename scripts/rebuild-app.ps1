# 学习平台 仅重建前后端脚本（中间件不动）
# 用途：改了 backend/ 或 frontend/ 的代码后，只重建这两个应用镜像并重启。
#       MySQL / Redis / MinIO 是 image 服务（不需要构建），不碰它们，
#       比 .\start.ps1 全量流程更快。
# 用法：
#   .\rebuild-app.ps1          # 重建并重启 backend + frontend
#   .\rebuild-app.ps1 -NoBrowser   # 启动后不自动打开浏览器

param(
    [switch]$NoBrowser
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$EnvFile = Join-Path $ProjectRoot ".env"

function Get-EnvValue([string]$Key) {
    $Line = Get-Content -LiteralPath $EnvFile | Where-Object { $_ -match "^\s*$Key\s*=" } | Select-Object -First 1
    if ($Line) {
        return ($Line -split "=", 2)[1].Trim()
    }
    return $null
}

$HttpPort = Get-EnvValue "HTTP_PORT"
if (-not $HttpPort) { $HttpPort = "8088" }
$BackendPort = Get-EnvValue "BACKEND_PORT"
if (-not $BackendPort) { $BackendPort = "8888" }

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  仅重建 backend + frontend" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

Set-Location -LiteralPath $ProjectRoot

# 1. 只构建前后端两个应用镜像（mysql/redis/minio 不受影响）
Write-Host "正在重建 backend + frontend 镜像（依赖层会命中缓存）..." -ForegroundColor Cyan
docker compose build backend frontend
if ($LASTEXITCODE -ne 0) {
    Write-Host "镜像构建失败，请查看上面的报错" -ForegroundColor Red
    exit 1
}

# 2. 只重启前后端容器（中间件容器不动，数据不丢）
Write-Host "正在重启 backend + frontend 容器 ..." -ForegroundColor Cyan
docker compose up -d backend frontend
if ($LASTEXITCODE -ne 0) {
    Write-Host "容器启动失败，请查看上面的报错" -ForegroundColor Red
    exit 1
}

# 3. 等待后端接口就绪（最多 60 秒）
Write-Host "等待后端就绪 ..." -ForegroundColor Yellow
$ready = $false
for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Seconds 3
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$BackendPort/healthz" -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -eq 200) { $ready = $true; break }
    } catch { }
}
if (-not $ready) {
    Write-Host "后端未就绪，打印容器状态与日志：" -ForegroundColor Red
    docker compose ps
    docker compose logs --tail=60 backend
    exit 1
}

Write-Host ""
Write-Host "前端页面:  http://localhost:$HttpPort" -ForegroundColor Green
Write-Host "后端 API:  http://localhost:$BackendPort" -ForegroundColor Green

# 4. 打开浏览器（默认用 Microsoft Edge，找不到 Edge 时退回系统默认浏览器）
if (-not $NoBrowser) {
    $edge = "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
    if (-not (Test-Path -LiteralPath $edge)) {
        $edge = "${env:ProgramFiles}\Microsoft\Edge\Application\msedge.exe"
    }
    if (Test-Path -LiteralPath $edge) {
        Start-Process -FilePath $edge -ArgumentList "http://localhost:$HttpPort"
    } else {
        Start-Process "http://localhost:$HttpPort"
    }
}