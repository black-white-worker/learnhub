# 学习平台 一键启动脚本（参考 S:\videonest\scripts 的风格）
# 用法：
#   .\start.ps1              # 启动（默认自动检测：源码没变就跳过构建）
#   .\start.ps1 -ForceBuild  # 强制重新构建镜像
#   .\start.ps1 -SkipBuild   # 强制跳过构建，直接启动（更快）
#   .\start.ps1 -NoBrowser   # 启动后不自动打开浏览器

param(
    [switch]$SkipBuild,
    [switch]$ForceBuild,
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

# 统一执行 docker 命令：构建/清理时 docker 会把进度写到 stderr，
# PowerShell 5.1 在 $ErrorActionPreference="Stop" 下会把 stderr 当成
# NativeCommandError 直接中断脚本（表现：构建刚开始就报"Image ... Building"错误退出）。
# 这里用 $args 收参、把 stderr 并入正常输出、临时关闭"错误即终止"，只以退出码判断成败。
function Invoke-DockerCmd {
    $oldEAP = $ErrorActionPreference
    $oldNative = $null
    if ($PSNativeCommandUseErrorActionPreference -ne $null) {
        $oldNative = $PSNativeCommandUseErrorActionPreference
        $PSNativeCommandUseErrorActionPreference = $false
    }
    $ErrorActionPreference = "Continue"
    try {
        # 把 docker 输出逐行转成正常颜色打印：
        # 构建/启动时 docker 把进度写到 stderr，5.1 会用红色"错误样式"显示它
        # （看起来像报错，其实是正常日志）；逐行 Write-Host 后不再显示成错误。
        docker $args 2>&1 | ForEach-Object { Write-Host $_ }
    } finally {
        $ErrorActionPreference = $oldEAP
        if ($oldNative -ne $null) {
            $PSNativeCommandUseErrorActionPreference = $oldNative
        }
    }
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
$MinioApiPort = Get-EnvValue "MINIO_API_PORT"
if (-not $MinioApiPort) { $MinioApiPort = "9020" }
$MinioConsolePort = Get-EnvValue "MINIO_CONSOLE_PORT"
if (-not $MinioConsolePort) { $MinioConsolePort = "9021" }

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

# 4. 构建并启动容器（默认行为：源码没变就跳过构建；代码变化、首次运行或 -ForceBuild 才重建）
Set-Location -LiteralPath $ProjectRoot

# 源码指纹记录文件：放在用户目录，不污染项目 git 状态
$stateFile = Join-Path $env:LOCALAPPDATA "LearningPlatform\build-state.txt"

# 计算源码指纹：把影响镜像内容的文件全部哈希一遍（前端排除 node_modules/dist）
function Get-SourceHash {
    $files = @()
    foreach ($f in @("docker-compose.yml", ".env")) {
        $p = Join-Path $ProjectRoot $f
        if (Test-Path -LiteralPath $p) { $files += $p }
    }
    # 后端全部源码 + 前端 src 与根目录配置文件（Dockerfile/nginx.conf/package.json 等）
    $files += Get-ChildItem -LiteralPath (Join-Path $ProjectRoot "backend") -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName }
    $files += Get-ChildItem -LiteralPath (Join-Path $ProjectRoot "frontend\src") -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName }
    $files += Get-ChildItem -LiteralPath (Join-Path $ProjectRoot "frontend") -File -ErrorAction SilentlyContinue | ForEach-Object { $_.FullName }

    $digest = ($files | Sort-Object | ForEach-Object {
        (Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash
    }) -join "`n"
    return (Get-FileHash -InputStream ([IO.MemoryStream]::new([Text.Encoding]::UTF8.GetBytes($digest))) -Algorithm SHA256).Hash
}

# 判断是否需要构建：-ForceBuild 强制；否则对比上次构建记录的源码指纹
$needBuild = $ForceBuild
if (-not $needBuild) {
    $currentHash = Get-SourceHash
    $lastHash = $null
    if (Test-Path -LiteralPath $stateFile) {
        $lastHash = (Get-Content -LiteralPath $stateFile -Raw).Trim()
    }
    if ($lastHash -eq $currentHash) {
        $needBuild = $false
        Write-Host "源码没有变化，跳过构建，直接启动 ..." -ForegroundColor Green
    } else {
        $needBuild = $true
        Write-Host "源码有变化或首次运行，开始构建 ..." -ForegroundColor Cyan
    }
} else {
    Write-Host "检测到 -ForceBuild，强制重新构建 ..." -ForegroundColor Cyan
}

if ($SkipBuild) {
    Write-Host "跳过构建，直接启动容器 ..." -ForegroundColor Cyan
    Invoke-DockerCmd compose up -d
} elseif ($needBuild) {
    Invoke-DockerCmd compose up -d --build
} else {
    Invoke-DockerCmd compose up -d
}
if ($LASTEXITCODE -ne 0) {
    Write-Host "docker compose 启动失败，请查看上面的报错" -ForegroundColor Red
    exit 1
}

# 构建成功后记录本次源码指纹，下次没变化就跳过构建；构建失败不记录（下次会重试）
if ($needBuild -and -not $SkipBuild) {
    $stateDir = Split-Path -Parent $stateFile
    if (-not (Test-Path -LiteralPath $stateDir)) {
        New-Item -ItemType Directory -Path $stateDir | Out-Null
    }
    [IO.File]::WriteAllText($stateFile, (Get-SourceHash), [Text.Encoding]::UTF8)
    Write-Host "已记录本次构建指纹，下次源码没变化将跳过构建" -ForegroundColor DarkGray
}

# 4.1 清理旧构建的悬空镜像（dangling image）
# docker compose build 每次会生成新镜像，旧版本会变成"悬空镜像"堆积占磁盘。
# 这里只删没有任何标签/容器引用的悬空镜像，不影响正在使用的镜像，安全。
Write-Host "清理旧构建的悬空镜像 ..." -ForegroundColor Cyan
Invoke-DockerCmd image prune -f
if ($LASTEXITCODE -ne 0) {
    Write-Host "镜像清理失败（不影响服务运行，可忽略）" -ForegroundColor Yellow
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
    Invoke-DockerCmd compose ps
    Invoke-DockerCmd compose logs --tail=60 backend
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
Write-Host "  MinIO:     http://127.0.0.1:$MinioApiPort  (控制台 http://127.0.0.1:$MinioConsolePort  minioadmin/minioadmin)"
Write-Host "  常用命令:  docker compose logs -f backend（看后端日志）"
Write-Host "             docker compose down（停止服务）"
Write-Host "========================================" -ForegroundColor Green

# 7. 打开浏览器（默认用 Microsoft Edge，找不到 Edge 时退回系统默认浏览器）
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

# 8. 清理 24 小时前的 Docker 构建缓存（保留近 24 小时的缓存加速当天的重建）
# 说明：不要用 docker builder prune -f 全清——那样下次代码改动重建时要重新
# 下载基础镜像 + 重装依赖，反而更慢。实测 7 天窗口清不动（缓存条目都是
# 最近 7 天内创建的），24 小时窗口最合适：每次启动清掉"昨天及更早"的缓存，
# 让缓存量锁死在"最近一天的量"，不会无限堆积。
Write-Host "清理 24 小时前的构建缓存 ..." -ForegroundColor Cyan
Invoke-DockerCmd builder prune -f --filter until=24h
if ($LASTEXITCODE -ne 0) {
    Write-Host "构建缓存清理失败（不影响服务运行，可忽略）" -ForegroundColor Yellow
}
