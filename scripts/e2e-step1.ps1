$base = "http://127.0.0.1:8888"
$ErrorActionPreference = "Stop"

function Login($u, $p) {
    $body = @{username=$u;password=$p} | ConvertTo-Json
    return (Invoke-RestMethod -Uri "$base/api/user/login" -Method Post -ContentType "application/json" -Body $body).token
}
function MkHeaders($token) {
    return @{Authorization = "Bearer $token"}
}

Write-Host "===== 登录三个账号 ====="
$adminT = Login "admin" "123456"
$teacherT = Login "teacher" "teacher"
$studentT = Login "222" "222222"
Write-Host "admin token: $($adminT.Substring(0,16))..."
Write-Host "teacher token: $($teacherT.Substring(0,16))..."
Write-Host "student token: $($studentT.Substring(0,16))..."

Write-Host "`n===== 老师查看我教的班 ====="
$h = MkHeaders $teacherT
try {
    $myClasses = Invoke-RestMethod -Uri "$base/api/class/my" -Headers $h
    $myClasses | ConvertTo-Json -Depth 4
} catch {
    Write-Host "/api/class/my 失败: $($_.Exception.Message)"
    Write-Host "尝试 /api/class/all ..."
    $h2 = MkHeaders $adminT
    $allClasses = Invoke-RestMethod -Uri "$base/api/class/all" -Headers $h2
    $allClasses | ConvertTo-Json -Depth 4
}
