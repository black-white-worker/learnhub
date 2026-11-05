# 本地部署 / 调试脚本（只操作本机 Docker，和服务器部署无关）
#
# 分工说明：
#   docker-compose.yml     本地全栈：MySQL / Redis / MinIO / Flyway / backend / frontend，
#                          镜像在本机 Docker 里从源码构建，不经过服务器。
#   scripts\deploy.ps1      服务器部署：本机编译产物 → 上传 → 服务器只 COPY。
#   本脚本                 本地一键构建 / 启动 / 停止 / 看日志 / 只起中间件。
#
# 用法：
#   .\local.cmd                 构建并启动全套本地容器（源码没变会跳过构建）
#   .\local.cmd -ForceBuild     强制重建 backend + frontend 镜像
#   .\local.cmd -SkipBuild      不构建，直接启动已有镜像
#   .\local.cmd -Infra          只起中间件（MySQL/Redis/MinIO/Flyway），应用用热更新方式跑
#   .\local.cmd -Status         查看容器状态 + 后端健康
#   .\local.cmd -Logs           跟踪 backend + frontend 日志（Ctrl+C 退出）
#   .\local.cmd -Down           停止并删除容器（数据卷保留）
#   .\local.cmd -NoBrowser      启动后不自动打开浏览器
param(
    [switch]$SkipBuild,
    [switch]$ForceBuild,
    [switch]$NoBrowser,
    [switch]$Infra,
    [switch]$Status,
    [switch]$Logs,
    [switch]$Down
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$ComposeFile = Join-Path $ProjectRoot "docker-compose.yml"
$EnvFile = Join-Path $ProjectRoot ".env"
$EnvExample = Join-Path $ProjectRoot ".env.example"

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

# 执行 docker 命令：把 stderr 并入输出、临时关掉"错误即终止"，
# 避免 PowerShell 5.1 把 docker 的进度输出当成异常中断脚本（只按退出码判断）。
function Invoke-Docker {
    $oldEap = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        & docker @args 2>&1 | ForEach-Object { Write-Host $_ }
        return $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldEap
    }
}

# 执行 docker 命令并只取 stdout（用于 inspect 这类查询）；失败返回 $null
function Get-DockerOutput {
    $oldEap = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $out = & docker @args 2>$null
        if ($LASTEXITCODE -ne 0) { return $null }
        return (($out | Out-String).Trim())
    } finally {
        $ErrorActionPreference = $oldEap
    }
}

function Get-EnvValue([string]$Key) {
    if (-not (Test-Path -LiteralPath $EnvFile)) { return $null }
    $line = Get-Content -LiteralPath $EnvFile | Where-Object { $_ -match "^\s*$Key\s*=" } | Select-Object -First 1
    if ($line) { return (($line -split "=", 2)[1]).Trim() }
    return $null
}

if (-not (Test-Path -LiteralPath $ComposeFile)) {
    Write-Host "找不到 docker-compose.yml：$ComposeFile" -ForegroundColor Red
    exit 1
}

# .env 不存在时用模板生成，方便新克隆的仓库直接跑
if (-not (Test-Path -LiteralPath $EnvFile)) {
    if (Test-Path -LiteralPath $EnvExample) {
        Copy-Item -LiteralPath $EnvExample -Destination $EnvFile
        Write-Host "未找到 .env，已从 .env.example 生成（模板密码只适合本机，别拿去部署）" -ForegroundColor Yellow
    } else {
        Write-Host "未找到 .env，也没有 .env.example" -ForegroundColor Red
        exit 1
    }
}

Set-Location -LiteralPath $ProjectRoot

# 下面的操作都要 Docker 先跑起来；默认全套启动交给 start.ps1，
# 它自带"自动拉起 Docker Desktop"的逻辑，所以这里只拦非默认操作。
if ($null -eq (Get-DockerOutput info)) {
    if ($Down -or $Logs -or $Status -or $Infra) {
        Write-Host "Docker 引擎未运行：先打开 Docker Desktop，等状态变绿再执行。" -ForegroundColor Red
        exit 1
    }
}

if ($Down) {
    Write-Step "停止并删除本地容器（数据卷保留）"
    exit (Invoke-Docker compose down)
}

if ($Logs) {
    Write-Step "跟踪 backend + frontend 日志（Ctrl+C 退出）"
    exit (Invoke-Docker compose logs -f --tail=200 backend frontend)
}

if ($Status) {
    Write-Step "本地容器状态"
    $code = Invoke-Docker compose ps
    $backendPort = Get-EnvValue "BACKEND_PORT"
    if (-not $backendPort) { $backendPort = "8888" }
    Write-Host ""
    Write-Host "后端健康（http://127.0.0.1:$backendPort/healthz）：" -NoNewline
    try {
        $resp = Invoke-WebRequest -Uri "http://127.0.0.1:$backendPort/healthz" -UseBasicParsing -TimeoutSec 3
        Write-Host " $($resp.StatusCode)" -ForegroundColor Green
    } catch {
        Write-Host " 不通（容器可能没启动）" -ForegroundColor Yellow
    }
    exit $code
}

# 只起中间件：适合"后端 go run + 前端 npm run dev"的热更新调试
if ($Infra) {
    Write-Step "只启动中间件：mysql / redis / minio / minio-init / flyway"
    $code = Invoke-Docker compose up -d mysql redis minio minio-init flyway
    if ($code -ne 0) {
        Write-Host "中间件启动失败，请看上面的报错" -ForegroundColor Red
        exit $code
    }

    Write-Step "等待 Flyway 迁移完成（最多 120 秒）"
    $flywayContainer = "learningplatform-flyway"
    $migrated = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Seconds 3
        $state = Get-DockerOutput inspect -f "{{.State.Status}}" $flywayContainer
        if ($state -eq "exited") {
            $exitCode = Get-DockerOutput inspect -f "{{.State.ExitCode}}" $flywayContainer
            if ($exitCode -eq "0") { $migrated = $true; break }
            Write-Host "Flyway 迁移失败（exit $exitCode），最近日志：" -ForegroundColor Red
            Invoke-Docker compose logs --tail=60 flyway
            exit 1
        }
        Write-Host "  迁移进行中 $($i * 3 + 3)s / 120s"
    }
    if (-not $migrated) {
        Write-Host "Flyway 120 秒内未完成，查看日志：docker compose logs flyway" -ForegroundColor Red
        exit 1
    }

    $backendPort = Get-EnvValue "BACKEND_PORT"; if (-not $backendPort) { $backendPort = "8888" }
    $dbPort = Get-EnvValue "DB_PORT"; if (-not $dbPort) { $dbPort = "3316" }
    $redisPort = Get-EnvValue "REDIS_PORT"; if (-not $redisPort) { $redisPort = "6381" }
    $minioPort = Get-EnvValue "MINIO_API_PORT"; if (-not $minioPort) { $minioPort = "9020" }
    $minioConsolePort = Get-EnvValue "MINIO_CONSOLE_PORT"; if (-not $minioConsolePort) { $minioConsolePort = "9021" }

    Write-Host ""
    Write-Host "中间件已就绪：" -ForegroundColor Green
    Write-Host "  MySQL   127.0.0.1:$dbPort"
    Write-Host "  Redis   127.0.0.1:$redisPort"
    Write-Host "  MinIO   http://127.0.0.1:$minioPort （控制台 http://127.0.0.1:$minioConsolePort）"
    Write-Host ""
    Write-Host "接下来用热更新方式跑应用：" -ForegroundColor Cyan
    Write-Host "  后端：.\scripts\start-backend.ps1     # go run，改完代码重新执行"
    Write-Host "  前端：cd frontend; npm run dev       # Vite HMR，http://localhost:5173"
    Write-Host "  后端地址：http://127.0.0.1:$backendPort"
    exit 0
}

# 默认：构建并启动全套本地容器。构建/指纹/健康检查逻辑复用 start.ps1，避免两套实现。
$startScript = Join-Path $ScriptDir "start.ps1"
if (-not (Test-Path -LiteralPath $startScript)) {
    Write-Host "找不到 scripts\start.ps1" -ForegroundColor Red
    exit 1
}

$startArgs = @{}
if ($SkipBuild) { $startArgs["SkipBuild"] = $true }
if ($ForceBuild) { $startArgs["ForceBuild"] = $true }
if ($NoBrowser) { $startArgs["NoBrowser"] = $true }

Write-Step "本地构建并启动全套容器（docker-compose.yml）"
& $startScript @startArgs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
exit 0
