# 学习平台 loopback 压测（2026-11-09）

参考 videonest 2026-11-07 harness 的方法：把 Node.js 压测脚本发到服务器 `/tmp/lpperf-20261109/`，用 `docker run --network host` 在容器内打 `127.0.0.1:8888`，绕过公网带宽限制，测服务端真实处理能力。

## 文件清单

| 文件 | 作用 |
|------|------|
| perfload.js | 核心压测驱动器（无第三方依赖），支持 simple/login/note-add/progress 场景 |
| bootstrap.js | 注册 4 个探针账号 lpperf_1~4，登录拿 token 写 tokens.json |
| run_all.sh | 分块编排：A=公开读 / B=登录 / C=鉴权中间件 / D=写接口 |
| summarize.js | 汇总 raw/*.json 输出 markdown 表格 |

## 服务器环境

- 目标：`http://127.0.0.1:8888`（lp-backend 容器，go-zero）
- 服务器：101.201.208.223，2 vCPU / 1.8G 内存（与 videonest、heart-pilot 共享）
- 内存护栏：MemAvailable < 45MB 自动停止发压
- 邻居金丝雀：lp:80 / hp:8081 / vn:8082 连续 2 次非 200 自动停止

## 压测接口

### A 块：公开只读（无需登录）
- GET /api/course/list?page=1&pageSize=20（课程列表，唯一一条课程）
- GET /api/course/detail/5（课程详情）
- 并发阶梯：8 / 16 / 32 / 64 / 128，每档 30s

### B 块：登录接口
- POST /api/user/login（bcrypt CPU 密集）
- 并发阶梯：4 / 8 / 16，每档 30s

### C 块：鉴权中间件开销（带 token，空列表接口）
- GET /api/user/info
- GET /api/class/my
- GET /api/notifications?page=1&pageSize=20
- GET /api/my/exams
- GET /api/my/assignments
- GET /api/class/all
- 并发阶梯：8 / 16 / 32 / 64，每档 30s

### D 块：写接口（小并发，控制污染）
- POST /api/course/note（写笔记）
- POST /api/course/video/progress/5/1（保存播放进度）
- 并发阶梯：2 / 4 / 8，每档 30s

## 执行步骤（在服务器上）

```bash
# 1. 上传脚本后
mkdir -p /tmp/lpperf-20261109/raw
cd /tmp/lpperf-20261109
# 把 perfload.js / bootstrap.js / run_all.sh / summarize.js 放进来

# 2. 建探针账号（注册限流 5次/分钟/IP，一次跑 4 个）
docker run --rm --network host -v /tmp/lpperf-20261109:/w node:24-alpine \
  node /w/bootstrap.js /w/tokens.json lpperf

# 3. 分块跑（每块之间观察内存）
bash run_all.sh A
bash run_all.sh B
bash run_all.sh C
bash run_all.sh D

# 4. 汇总
docker run --rm -v /tmp/lpperf-20261109:/w node:24-alpine node /w/summarize.js /w/raw > /w/summary.md
```

## 注意事项

- 注册接口限流 5次/分钟/IP，bootstrap 一次只注册 4 个账号，重复跑会被 429 拦住
- 服务器内存极紧（available ~115MB），压测容器限制 128m，护栏 45MB
- 写接口（note-add）会往 video_notes 表插数据，测试后需清理 `content LIKE 'LP20261109%'`
- 全程只读/小量写，不影响线上核心数据
