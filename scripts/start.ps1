# 学习平台 一键启动脚本（参考 S:\videonest\scripts 的风格）
# 用法：
#   .\start.ps1                    # 启动（默认自动检测：源码没变就跳过构建）
#   .\start.ps1 -ForceBuild        # 强制重新构建镜像
#   .\start.ps1 -SkipBuild         # 强制跳过构建，直接启动（更快）
#   .\start.ps1 -NoBrowser         # 启动后不自动打开浏览器
#   .\start.ps1 -HealthTimeout 120 # 修改后端健康检测超时（单位秒，默认90）
#   .\start.ps1 -HealthUrl "http://127.0.0.1:8888/healthz"
#   .\start.ps1 -Quiet             # 精简日志，减少刷屏

param(
    [switch]$SkipBuild,
    [switch]$ForceBuild,
    [switch]$NoBrowser,
    [int]$HealthTimeout = 90,
    [string]$HealthUrl = $null,
    [switch]$Quiet
)

$ErrorActionPreference = "Stop"

# 脚本位于 scripts/ 下，项目根目录是脚本所在目录的上一级
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$EnvFile = Join-Path $ProjectRoot ".env"
$ComposeFile = Join-Path $ProjectRoot "docker-compose.yml"
$ScriptStateVersion = "v2" # 脚本哈希算法版本，升级脚本自动作废旧指纹

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  学习平台 一键启动" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 日志封装，兼容 -Quiet
function Write-Log {
    param(
        [Parameter(Mandatory=$true)]
        [string]$Message,
        [ValidateSet("Info","Warn","Error")]
        [string]$Level = "Info"
    )
    if ($Quiet -and $Level -eq "Info") { return }
    $color = switch($Level) {
        "Warn"  { "Yellow" }
        "Error" { "Red" }
        default { "White" }
    }
    Write-Host $Message -ForegroundColor $color
}

# 从 .env 读取一个键的值（取第一个匹配行，去掉行内注释）
function Get-EnvValue([string]$Key) {
    if (-not (Test-Path -LiteralPath $EnvFile)) { return $null }
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
        docker $args 2>&1 | ForEach-Object { Write-Host $_ }
        return $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldEAP
        if ($oldNative -ne $null) {
            $PSNativeCommandUseErrorActionPreference = $oldNative
        }
    }
}

# 1. 前置文件检查
if (-not (Test-Path -LiteralPath $EnvFile)) {
    Write-Log "未找到 .env 文件：$EnvFile（请确认项目文件完整）" -Level Error
    exit 1
}
if (-not (Test-Path -LiteralPath $ComposeFile)) {
    Write-Log "未找到 docker-compose.yml：$ComposeFile" -Level Error
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

# 健康地址优先级：传参 > 默认
if (-not $HealthUrl) {
    $HealthUrl = "http://127.0.0.1:$BackendPort/healthz"
}

# 3. 检查 Docker 引擎是否可用；不可用时自动拉起 Docker Desktop 并等待
$dockerOk = $false
try {
    $ec = Invoke-DockerCmd info
    if ($ec -eq 0) { $dockerOk = $true }
} catch { }

if (-not $dockerOk) {
    Write-Log "Docker 引擎未运行，正在启动 Docker Desktop ..." -Level Warn
    $dockerDesktop = @(
        "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe",
        "$env:LOCALAPPDATA\Programs\DockerDesktop\Docker Desktop.exe",
        "$env:LOCALAPPDATA\Docker\Docker Desktop.exe"
    ) | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    if (-not $dockerDesktop) {
        Write-Log "找不到 Docker Desktop，请先安装并启动 Docker，再运行本脚本" -Level Error
        exit 1
    }
    Start-Process -FilePath $dockerDesktop

    Write-Log "等待 Docker 引擎就绪（最多 120 秒，Ctrl+C 退出）..." -Level Warn
    $ready = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Seconds 3
        try {
            $ec = Invoke-DockerCmd info
            if ($ec -eq 0) { $ready = $true; break }
        } catch { }
        Write-Host "  等待Docker就绪中 $($i*3)s / 120s"
    }
    if (-not $ready) {
        Write-Log "Docker 引擎 120 秒内未就绪，请手动检查 Docker Desktop" -Level Error
        exit 1
    }
    Write-Log "Docker 引擎已就绪。"
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
    $backFiles = Get-ChildItem -LiteralPath (Join-Path $ProjectRoot "backend") -Recurse -File -ErrorAction SilentlyContinue
    $frontSrcFiles = Get-ChildItem -LiteralPath (Join-Path $ProjectRoot "frontend\src") -Recurse -File -ErrorAction SilentlyContinue
    $frontRootFiles = Get-ChildItem -LiteralPath (Join-Path $ProjectRoot "frontend") -File -ErrorAction SilentlyContinue
    $allCandidate = @($backFiles, $frontSrcFiles, $frontRootFiles) | Where-Object { $_ }

    foreach ($item in $allCandidate) {
        foreach ($f in $item) {
            try {
                $files += $f.FullName
            } catch {
                # 跳过锁定/无权限文件，不中断哈希计算
                continue
            }
        }
    }

    $digestLines = @($ScriptStateVersion)
    $digestLines += ($files | Sort-Object | ForEach-Object {
        try {
            (Get-FileHash -LiteralPath $_ -Algorithm SHA256 -ErrorAction Stop).Hash
        } catch {
            $null
        }
    })
    $digest = $digestLines -join "`n"
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
        Write-Log "源码没有变化，跳过构建，直接启动 ..."
    } else {
        $needBuild = $true
        Write-Log "源码有变化或首次运行，开始构建 ..."
    }
} else {
    Write-Log "检测到 -ForceBuild，强制重新构建 ..."
}

if ($SkipBuild) {
    Write-Log "跳过构建，直接启动容器 ..."
    $ec = Invoke-DockerCmd compose up -d
} elseif ($needBuild) {
    $ec = Invoke-DockerCmd compose up -d --build
} else {
    $ec = Invoke-DockerCmd compose up -d
}
if ($ec -ne 0) {
    Write-Log "docker compose 启动失败，请查看上面的报错" -Level Error
    exit 1
}

# 构建成功后记录本次源码指纹，下次没变化就跳过构建；构建失败不记录（下次会重试）
if ($needBuild -and -not $SkipBuild) {
    $stateDir = Split-Path -Parent $stateFile
    if (-not (Test-Path -LiteralPath $stateDir)) {
        New-Item -ItemType Directory -Path $stateDir | Out-Null
    }
    [IO.File]::WriteAllText($stateFile, (Get-SourceHash), [Text.Encoding]::UTF8)
    Write-Log "已记录本次构建指纹，下次源码没变化将跳过构建"
}

# 4.1 清理旧构建的悬空镜像（dangling image）
# docker compose build 每次会生成新镜像，旧版本会变成"悬空镜像"堆积占磁盘。
# 这里只删没有任何标签/容器引用的悬空镜像，不影响正在使用的镜像，安全。
Write-Log "清理旧构建的悬空镜像 ..."
$ec = Invoke-DockerCmd image prune -f
if ($ec -ne 0) {
    Write-Log "镜像清理失败（不影响服务运行，可忽略）" -Level Warn
}

# 5. 等待后端接口就绪（可配置超时）
Write-Log "等待服务就绪（最长 ${HealthTimeout}s，检测地址：$HealthUrl）..." -Level Warn
$ready = $false
$round = [math]::Floor($HealthTimeout / 3)
for ($i = 0; $i -lt $round; $i++) {
    Start-Sleep -Seconds 3
    try {
        $resp = Invoke-WebRequest -Uri $HealthUrl -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -eq 200) {
            $ready = $true
            break
        }
    } catch {
        Write-Host "  后端未就绪，等待中 $($i*3+3)s / ${HealthTimeout}s"
    }
}
if (-not $ready) {
    Write-Log "服务未就绪，打印容器状态与后端日志：" -Level Error
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
Write-Host "  测试账号:  admin / 123456（首次登录需修改密码）"
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
        Write-Log "使用 Edge 打开前端页面"
        Start-Process -FilePath $edge -ArgumentList "http://localhost:$HttpPort"
    } else {
        Write-Log "使用系统默认浏览器打开前端页面"
        Start-Process "http://localhost:$HttpPort"
    }
}

# 8. 清理 24 小时前的 Docker 构建缓存（保留近 24 小时的缓存加速当天的重建）
# 说明：不要用 docker builder prune -f 全清——那样下次代码改动重建时要重新
# 下载基础镜像 + 重装依赖，反而更慢。实测 7 天窗口清不动（缓存条目都是
# 最近 7 天内创建的），24 小时窗口最合适：每次启动清掉"昨天及更早"的缓存，
# 让缓存量锁死在"最近一天的量"，不会无限堆积。
Write-Log "清理 24 小时前的构建缓存（保留当日构建缓存加速）..."
$ec = Invoke-DockerCmd builder prune -f --filter until=24h
if ($ec -ne 0) {
    Write-Log "构建缓存清理失败（不影响服务运行，可忽略）" -Level Warn
}
