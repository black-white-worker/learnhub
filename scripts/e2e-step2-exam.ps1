$base = "http://127.0.0.1:8888"
$ErrorActionPreference = "Stop"

function Login($u, $p) {
    $body = @{username=$u;password=$p} | ConvertTo-Json
    return (Invoke-RestMethod -Uri "$base/api/user/login" -Method Post -ContentType "application/json" -Body $body).token
}
function MkHeaders($token) { return @{Authorization = "Bearer $token"} }
function NowStr($offsetMin) {
    return (Get-Date).AddMinutes($offsetMin).ToString("yyyy-MM-ddTHH:mm")
}

$teacherT = Login "teacher" "teacher"
$studentT = Login "222" "222222"
$hT = MkHeaders $teacherT
$hS = MkHeaders $studentT

Write-Host "===== 1. teacher creates exam in class 3 ====="
$examBody = @{
    title = "E2E Test Exam"
    description = "Automated test"
    startAt = NowStr (-5)
    endAt = NowStr 60
    durationMinutes = 30
    totalScore = 100
} | ConvertTo-Json
$exam = Invoke-RestMethod -Uri "$base/api/class/3/exam" -Method Post -ContentType "application/json" -Headers $hT -Body $examBody
$examId = $exam.id
Write-Host "exam created, id=$examId"

Write-Host "`n===== 2. teacher batch-adds 5 questions ====="
$questions = @(
    @{type="single";   stem="1+1=?";                         options='["1","2","3","4"]';    answer="B";     score=20},
    @{type="multiple"; stem="Which are Go keywords?";        options='["func","var","python","package"]'; answer="A,B,D"; score=20},
    @{type="judge";    stem="Go has GC";                     options='["true","false"]';      answer="A";     score=20},
    @{type="single";   stem="HTTP 404 means?";               options='["ok","not found","server err","redirect"]'; answer="B"; score=20},
    @{type="essay";    stem="Describe go-zero";               options='';                       answer='';      score=20}
)
$batchBody = @{questions=$questions} | ConvertTo-Json -Depth 5
$batch = Invoke-RestMethod -Uri "$base/api/exam/$examId/questions/batch" -Method Post -ContentType "application/json" -Headers $hT -Body $batchBody
Write-Host "batch add: $($batch.count) questions, ids=$($batch.ids -join ',')"

Write-Host "`n===== 3. student views detail (answers should be blanked) ====="
$detail = Invoke-RestMethod -Uri "$base/api/exam/$examId" -Headers $hS
Write-Host "question count: $($detail.questions.Count)"
foreach ($q in $detail.questions) {
    Write-Host "  Q$($q.id) [$($q.type)] $($q.stem) | answer=[$($q.answer)]"
}

Write-Host "`n===== 4. student starts exam ====="
$start = Invoke-RestMethod -Uri "$base/api/exam/$examId/start" -Method Post -Headers $hS
Write-Host "start resp: $($start | ConvertTo-Json -Compress)"

Write-Host "`n===== 5. student submits (intentionally: 3 correct, 1 wrong, 1 essay) ====="
$answers = @{
    ($batch.ids[0].ToString()) = "B"
    ($batch.ids[1].ToString()) = "A,B,D"
    ($batch.ids[2].ToString()) = "A"
    ($batch.ids[3].ToString()) = "A"
    ($batch.ids[4].ToString()) = "go-zero is a microservice framework"
}
$submitBody = @{answers=$answers} | ConvertTo-Json -Depth 5
$submit = Invoke-RestMethod -Uri "$base/api/exam/$examId/submit" -Method Post -ContentType "application/json" -Headers $hS -Body $submitBody
Write-Host "submit resp: $($submit | ConvertTo-Json -Compress)"
Write-Host ">>> expected autoScore = 60 (Q1+Q2+Q3, Q4 wrong)"

Write-Host "`n===== 6. teacher views submissions ====="
$detail2 = Invoke-RestMethod -Uri "$base/api/exam/$examId" -Headers $hT
Write-Host "submission count: $($detail2.submissions.Count)"
foreach ($s in $detail2.submissions) {
    Write-Host "  student $($s.username) autoScore=$($s.autoScore) teacherScore=$($s.teacherScore)"
}

Write-Host "`n===== 7. teacher grades essay (sets total 75) ====="
$sid = $detail2.submissions[0].id
$gradeBody = @{teacherScore=75; comment="essay ok, +15"} | ConvertTo-Json
Invoke-RestMethod -Uri "$base/api/exam/submission/$sid/grade" -Method Post -ContentType "application/json" -Headers $hT -Body $gradeBody | Out-Null
Write-Host "graded"

Write-Host "`n===== 8. final check ====="
$detail3 = Invoke-RestMethod -Uri "$base/api/exam/$examId" -Headers $hT
foreach ($s in $detail3.submissions) {
    Write-Host "  student $($s.username) autoScore=$($s.autoScore) teacherScore=$($s.teacherScore) comment=$($s.comment)"
}
