# Deploy learningplatform to the shared-infra server (artifact mode).
#
# 之前：把整份源码打包丢给服务器，在 Docker 里跑 go build + npm ci + vite build。
#       服务器只有 2 vCPU / 2 GiB，一轮十几分钟，还经常卡在模块下载上。
# 现在：本机编译 linux 二进制 + npm run build → 只上传 ~25MB 产物
#       → 服务器仅 COPY 进瘦镜像，构建 10~30 秒。
#
# 说明：后端是 Go（不是 jar），所以产物是 backend\bin\learningplatform-api，
#       服务器侧对应 backend\Dockerfile.bin；前端对应 frontend\Dockerfile.dist。
#       两者都复用 Dockerfile.base 预生成的运行基础镜像（LibreOffice + ffmpeg）。
#
# What it does:
#   1) 本机 go build（GOOS=linux GOARCH=amd64 CGO_ENABLED=0，-trimpath -s -w）
#   2) 本机 npm ci（缺 node_modules 时）+ npm run build
#   3) 把产物 / Dockerfile / compose / redeploy.sh 打成一个 tar.gz 传到服务器
#   4) 服务器执行 deploy/server/redeploy.sh：确保 base 镜像 → COPY 产物建镜像 → up -d → 等健康
#
# Usage:
#   .\deploy.cmd                     # 标准部署（本机构建 + 上传 + 服务器重建）
#   .\deploy.cmd -SkipBuild          # 复用 backend\bin\learningplatform-api 与 frontend\dist
#   .\deploy.cmd -SkipUpload         # 不传产物，只用服务器上已有的产物重跑一遍
#   .\deploy.cmd -RebuildBase        # 服务器强制重建基础镜像（改了 Dockerfile.base 时）
#   .\deploy.cmd -SetupMirror        # 先配 Docker 镜像加速（幂等），再走标准部署
#   .\deploy.cmd -MirrorOnly         # 只配镜像加速，不编译、不上传、不重启应用
#   .\deploy.cmd -SetupMirror -AliyunMirror https://xxxx.mirror.aliyuncs.com
param(
    [string]$Server = "101.201.208.223",
    [string]$RemoteUser = "root",
    [string]$RemoteDir = "/opt/apps/learningplatform",
    [string]$IdentityFile,
    [string]$AliyunMirror = $env:ALIYUN_MIRROR_URL,
    [switch]$SetupMirror,
    [switch]$MirrorOnly,
    [switch]$SkipBuild,
    [switch]$SkipUpload,
    [switch]$RebuildBase
)

$ErrorActionPreference = "Stop"

$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Target = "$RemoteUser@$Server"
$WorkRoot = if ($env:PUBLIC) { $env:PUBLIC } else { $env:TEMP }
$Stage = Join-Path $WorkRoot "learningplatform-deploy-stage"
$Archive = Join-Path $WorkRoot "learningplatform-deploy.tar.gz"
$BinName = "learningplatform-api"
$BinPath = Join-Path $ProjectRoot "backend\bin\$BinName"
$BuildStateFile = Join-Path $ProjectRoot "backend\bin\.deploy-build-state"

$SshArgs = @('-o', 'ConnectTimeout=15')
if ($IdentityFile) {
    $SshArgs += @('-i', (Resolve-Path -LiteralPath $IdentityFile).Path)
}

function Invoke-OnServer {
    param([string]$Command)
    & ssh @SshArgs $Target $Command
    if ($LASTEXITCODE -ne 0) { throw "Remote command failed (exit $LASTEXITCODE): $Command" }
}

Write-Host "Target : $Target"
Write-Host "Remote : $RemoteDir"
Write-Host ""

# ---------------------------------------------------------------------
# 0) 连通性 / docker / env 检查（先失败，省得白编译一遍）
# ---------------------------------------------------------------------
& ssh @SshArgs $Target "echo CONNECT_OK" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Cannot connect to $Target (check SSH key and security group)." }

$DockerOk = ((& ssh @SshArgs $Target "command -v docker >/dev/null 2>&1 && echo YES || echo NO") | Out-String).Trim()
if ($DockerOk -ne "YES") {
    throw "Docker is not installed on the server. Run first: bash /opt/apps/learningplatform/deploy/server/bootstrap.sh"
}

$EnvOk = ((& ssh @SshArgs $Target "test -f '$RemoteDir/.env.prod' && echo YES || echo NO") | Out-String).Trim()
if ($EnvOk -ne "YES") {
    throw "$RemoteDir/.env.prod is missing on the server. Create it before deploying."
}

$Infra = ((& ssh @SshArgs $Target "docker ps --format '{{.Names}}' | grep -c '^infra-mysql$'") | Out-String).Trim()
if ($Infra -ne "1") {
    Write-Warning "infra-mysql 未运行，flyway 会失败。先执行：bash /opt/apps/learningplatform/deploy/server/deploy.sh infra"
}

# ---------------------------------------------------------------------
# 0.5) 可选：配置 Docker 镜像加速（幂等合并 daemon.json）
#      registry-mirrors 只影响 Docker Hub 的拉取；脚本里显式写的镜像站拉取不受影响。
# ---------------------------------------------------------------------
if ($SetupMirror -or $MirrorOnly) {
    if ($AliyunMirror -and $AliyunMirror -notmatch '^https?://[A-Za-z0-9\-\.:/]+$') {
        throw "AliyunMirror 不是合法 URL：$AliyunMirror"
    }

    $MirrorScript = Join-Path $ProjectRoot "deploy\server\setup-mirror.sh"
    if (-not (Test-Path -LiteralPath $MirrorScript)) { throw "缺少 deploy\server\setup-mirror.sh" }

    # 先单独传一份到 /tmp：这样 -MirrorOnly 不用等打包
    & scp @SshArgs $MirrorScript "${Target}:/tmp/lp-setup-mirror.sh"
    if ($LASTEXITCODE -ne 0) { throw "上传 setup-mirror.sh 失败" }

    $MirrorCmd = "sed -i 's/\r`$//' /tmp/lp-setup-mirror.sh; bash /tmp/lp-setup-mirror.sh"
    if ($AliyunMirror) { $MirrorCmd += " --aliyun '$AliyunMirror'" }
    Invoke-OnServer $MirrorCmd

    if ($MirrorOnly) {
        Write-Host "只配置镜像加速，结束（未编译、未上传产物、未重启应用）。"
        exit 0
    }
    Write-Host ""
}

$Total = [Diagnostics.Stopwatch]::StartNew()

# ---------------------------------------------------------------------
# 1) 本机编译后端（交叉编译到 linux/amd64，和服务器架构一致）
# ---------------------------------------------------------------------
if (-not $SkipBuild) {
    # 后端源码/依赖/构建脚本未变且产物存在时复用上次二进制，避免重复交叉编译。
    $BackendDir = Join-Path $ProjectRoot "backend"
    $BuildInputs = @(
        (Join-Path $BackendDir "go.mod"),
        (Join-Path $BackendDir "go.sum"),
        (Join-Path $BackendDir "Dockerfile.bin")
    )
    $BuildInputs += Get-ChildItem -LiteralPath $BackendDir -Recurse -File |
        Where-Object { $_.FullName -notmatch '[\\/](bin|\.git)[\\/]' } |
        Select-Object -ExpandProperty FullName
    $BuildInputs = $BuildInputs | Sort-Object -Unique
    $InputHash = ($BuildInputs | ForEach-Object {
        if (Test-Path -LiteralPath $_) { "$_=$((Get-FileHash -LiteralPath $_ -Algorithm SHA256).Hash)" }
    }) -join "`n"
    $BackendHash = (Get-FileHash -InputStream ([IO.MemoryStream]::new([Text.Encoding]::UTF8.GetBytes($InputHash))) -Algorithm SHA256).Hash
    $CachedHash = if (Test-Path -LiteralPath $BuildStateFile) { (Get-Content -LiteralPath $BuildStateFile -Raw).Trim() } else { "" }
    $ReuseBackend = (Test-Path -LiteralPath $BinPath) -and ($CachedHash -eq $BackendHash)
    if ($ReuseBackend) { Write-Host "[1/5] 后端源码未变化，复用已有 Linux 二进制" }
    else { Write-Host "[1/5] 本机编译 backend 二进制" }

    if (-not $ReuseBackend) {
    $Go = $null
    foreach ($candidate in @('go.exe', 'go')) {
        if (Get-Command $candidate -ErrorAction SilentlyContinue) { $Go = $candidate; break }
    }
    if (-not $Go) {
        foreach ($candidate in @("$env:LOCALAPPDATA\Programs\Go\bin\go.exe", "C:\Go\bin\go.exe", "S:\go\bin\go.exe")) {
            if (Test-Path -LiteralPath $candidate) { $Go = $candidate; break }
        }
    }
    if (-not $Go) {
        throw "找不到 go。需要 go 1.27.x（和 backend\go.mod 一致）并加入 PATH。"
    }

    New-Item -ItemType Directory -Force -Path (Split-Path $BinPath) | Out-Null
    if (Test-Path -LiteralPath $BinPath) { Remove-Item -LiteralPath $BinPath -Force }

    $SavedGoOS = $env:GOOS
    $SavedGoArch = $env:GOARCH
    $SavedCgo = $env:CGO_ENABLED
    $SavedGoProxy = $env:GOPROXY
    $sw = [Diagnostics.Stopwatch]::StartNew()
    Push-Location $BackendDir
    try {
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        if (-not $env:GOPROXY) { $env:GOPROXY = "https://goproxy.cn,direct" }
        & $Go build -trimpath -ldflags "-s -w" -o $BinPath ./cmd/api
        if ($LASTEXITCODE -ne 0) { throw "go build 失败（exit $LASTEXITCODE）" }
    } finally {
        Pop-Location
        $env:GOOS = $SavedGoOS
        $env:GOARCH = $SavedGoArch
        $env:CGO_ENABLED = $SavedCgo
        $env:GOPROXY = $SavedGoProxy
    }
    Write-Host ("      二进制完成，{0:N1} MB，用时 {1:N0}s" -f ((Get-Item $BinPath).Length / 1MB), $sw.Elapsed.TotalSeconds)
    [IO.File]::WriteAllText($BuildStateFile, $BackendHash, [Text.Encoding]::UTF8)
    }

    # -----------------------------------------------------------------
    # 2) 本机构建前端
    # -----------------------------------------------------------------
    Write-Host "[2/5] 本机构建 frontend dist"
    $sw.Restart()

    $Npm = $null
    foreach ($candidate in @('npm.cmd', 'npm')) {
        if (Get-Command $candidate -ErrorAction SilentlyContinue) { $Npm = $candidate; break }
    }
    if (-not $Npm) { throw "找不到 npm。" }

    $FrontendDir = Join-Path $ProjectRoot "frontend"
    if (-not (Test-Path (Join-Path $FrontendDir "node_modules\.bin\vite.cmd"))) {
        & $Npm --prefix $FrontendDir ci --no-audit --no-fund --registry https://registry.npmmirror.com
        if ($LASTEXITCODE -ne 0) { throw "npm ci 失败（exit $LASTEXITCODE）" }
    }
    & $Npm --prefix $FrontendDir run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build 失败（exit $LASTEXITCODE）" }
    Write-Host ("      dist 完成，用时 {0:N0}s" -f $sw.Elapsed.TotalSeconds)
}
else {
    Write-Host "[1/5] -SkipBuild：复用 backend\bin\$BinName"
    Write-Host "[2/5] -SkipBuild：复用 frontend\dist"
}

# ---------------------------------------------------------------------
# 3) 校验产物 + 组装上传目录
# ---------------------------------------------------------------------
if (-not (Test-Path -LiteralPath $BinPath)) { throw "backend\bin\$BinName 不存在，请去掉 -SkipBuild 重新部署。" }
$BinMb = (Get-Item $BinPath).Length / 1MB
if ($BinMb -lt 1) { throw "backend\bin\$BinName 只有 $BinMb MB，看起来不是完整的 linux 二进制。" }

$Dist = Join-Path $ProjectRoot "frontend\dist"
if (-not (Test-Path (Join-Path $Dist "index.html"))) { throw "frontend\dist\index.html 不存在，请去掉 -SkipBuild 重新部署。" }

Write-Host ("[3/5] 组装产物（二进制 {0:N1} MB + dist）" -f $BinMb)

if (Test-Path $Stage) { Remove-Item $Stage -Recurse -Force }
$StageDirs = @(
    "backend\bin",
    "backend\etc",
    "backend\deploy\sql",
    "frontend",
    "deploy\server\nginx"
)
foreach ($dir in $StageDirs) {
    New-Item -ItemType Directory -Force -Path (Join-Path $Stage $dir) | Out-Null
}

Copy-Item $BinPath (Join-Path $Stage "backend\bin\$BinName") -Force
Copy-Item (Join-Path $ProjectRoot "backend\etc\learningplatform-api.yaml") (Join-Path $Stage "backend\etc\learningplatform-api.yaml") -Force
Copy-Item (Join-Path $ProjectRoot "backend\deploy\sql\migration") (Join-Path $Stage "backend\deploy\sql\migration") -Recurse -Force
Copy-Item $Dist (Join-Path $Stage "frontend\dist") -Recurse -Force

$UploadFiles = @(
    "backend\Dockerfile.bin",
    "backend\Dockerfile.bin.dockerignore",
    "backend\Dockerfile.base",
    "backend\Dockerfile.base.dockerignore",
    "frontend\Dockerfile.dist",
    "frontend\Dockerfile.dist.dockerignore",
    "frontend\nginx.conf",
    "deploy\server\nginx\lp.conf",
    "deploy\server\pull-images.sh",
    "deploy\server\setup-mirror.sh",
    "deploy\server\redeploy.sh",
    "docker-compose.server.yml",
    "docker-compose.artifact.yml"
)
foreach ($relative in $UploadFiles) {
    $source = Join-Path $ProjectRoot $relative
    if (-not (Test-Path -LiteralPath $source)) { throw "缺少文件：$relative" }
    $destination = Join-Path $Stage $relative
    New-Item -ItemType Directory -Force -Path (Split-Path $destination) | Out-Null
    Copy-Item $source $destination -Force
}

# ---------------------------------------------------------------------
# 3b) 可选上传：共享中间件的定义与初始化脚本
# ---------------------------------------------------------------------
# 这三个文件不属于 lp-backend/lp-frontend 的构建产物，但三个项目都靠它们：
#   docker-compose.infra.yml      改了 Redis 淘汰策略 / MinIO CORS / 常驻内存下限
#   init/mysql/01-databases.sql   首次建库
#   init/mysql/02-app-users.sql   分项目账号（卷已初始化过，需要手动执行一次，见文件头）
# 只警告不 throw：少了它们应用层照样能部署，别让 infra 文件绑死每次应用部署。
$OptionalUploadFiles = @(
    "deploy\server\docker-compose.infra.yml",
    "deploy\server\bootstrap.sh",
    "deploy\server\deploy.sh"
)
foreach ($relative in $OptionalUploadFiles) {
    $source = Join-Path $ProjectRoot $relative
    if (Test-Path -LiteralPath $source) {
        $destination = Join-Path $Stage $relative
        New-Item -ItemType Directory -Force -Path (Split-Path $destination) | Out-Null
        Copy-Item $source $destination -Force
    }
    else {
        Write-Warning "可选文件缺失：$relative（服务器上的共享中间件定义将保持旧版本）"
    }
}

$InitSrc = Join-Path $ProjectRoot "deploy\server\init\mysql"
if (Test-Path -LiteralPath $InitSrc) {
    $InitDst = Join-Path $Stage "deploy\server\init\mysql"
    New-Item -ItemType Directory -Force -Path $InitDst | Out-Null
    Get-ChildItem -LiteralPath $InitSrc -File | ForEach-Object {
        Copy-Item $_.FullName -Destination (Join-Path $InitDst $_.Name) -Force
    }
    Write-Host ("      附带上传 init/mysql 迁移前脚本 {0} 个" -f (Get-ChildItem -LiteralPath $InitSrc -File).Count)
}
else {
    Write-Warning "可选目录缺失：deploy\server\init\mysql（首次建库/建账号脚本不会上传）"
}

# ---------------------------------------------------------------------
# 4) 打包 + 上传
# ---------------------------------------------------------------------
if ($SkipUpload) {
    Write-Host "[4/5] -SkipUpload：跳过上传（用服务器上已有的产物）"
}
else {
    Write-Host "[4/5] 上传产物"
    if (Test-Path $Archive) { Remove-Item $Archive -Force }
    Push-Location $Stage
    try {
        & tar -czf $Archive .
        if ($LASTEXITCODE -ne 0) { throw "打包失败。" }
    } finally {
        Pop-Location
    }

    $ArchiveMb = [math]::Round((Get-Item $Archive).Length / 1MB, 1)
    Write-Host "      压缩包 $ArchiveMb MB"

    $sw = [Diagnostics.Stopwatch]::StartNew()
    & scp @SshArgs $Archive "${Target}:/tmp/learningplatform-deploy.tar.gz"
    if ($LASTEXITCODE -ne 0) { throw "上传失败。" }

    # 只解包产物；不动服务器上的 .env.prod / 数据卷
    $RemoteExtract = "set -e; mkdir -p '{0}'; tar -xzf /tmp/learningplatform-deploy.tar.gz -C '{0}'; rm -f /tmp/learningplatform-deploy.tar.gz; sed -i 's/\r$//' '{0}/deploy/server/redeploy.sh' '{0}/deploy/server/setup-mirror.sh'; chmod +x '{0}/deploy/server/redeploy.sh' '{0}/deploy/server/setup-mirror.sh' '{0}/backend/bin/{1}'" -f $RemoteDir, $BinName
    Invoke-OnServer $RemoteExtract
    Write-Host ("      上传并解包完成，用时 {0:N0}s" -f $sw.Elapsed.TotalSeconds)
}

# ---------------------------------------------------------------------
# 5) 服务器重建 + 重启（只 COPY 产物）
# ---------------------------------------------------------------------
Write-Host "[5/5] 服务器重建（只 COPY 产物）"
if ($RebuildBase) {
    Invoke-OnServer "REBUILD_BASE=1 bash '$RemoteDir/deploy/server/redeploy.sh'"
}
else {
    Invoke-OnServer "bash '$RemoteDir/deploy/server/redeploy.sh'"
}

Write-Host ""
Write-Host ("Done in {0:N1} min: http://{1}/" -f $Total.Elapsed.TotalMinutes, $Server)
