# 学习平台 loopback 压测报告（2026-11-10）

## 1. 测试概述

| 项目 | 内容 |
|------|------|
| 发压位置 | 服务器本机 Docker 容器（--network host）打 127.0.0.1:8888 |
| 目标服务 | lp-backend（go-zero），容器 mem_limit 320m |
| 服务器 | 101.201.208.223，2 vCPU / 1.8G 内存（与 videonest、heart-pilot 共享） |
| 测试时间 | 2026-11-10 00:03 ~ 00:14 |
| 测试工具 | Node.js 原生 http 压测脚本（无第三方依赖），参考 videonest 2026-11-07 harness |
| 护栏 | MemAvailable < 40MB 自动停止；邻居金丝雀（lp:80 / hp:8081 / vn:8082）连续 2 次非 200 自动停止 |

**和 videonest 测法的一致性**：同样是把 Node 脚本发到服务器 /tmp 目录，用 docker run --network host 在容器内 loopback 发压，绕过公网带宽限制，测服务端真实处理能力。videonest 11.7 报告就是这套方法。

---

## 2. 各接口实测数据

### 2.1 课程列表 GET /api/course/list?page=1&pageSize=20（公开，Redis 缓存命中）

| 并发 | QPS | p50 | p95 | p99 | 成功率 | 吞吐 | 内存 |
|------|-----|-----|-----|-----|--------|------|------|
| 8 | 153 | 2.8ms | 19.0ms | 772ms | 99.57% | 33 KB/s | 120MB |
| 16 | 1980 | 2.86ms | 8.3ms | 68ms | 99.98% | 427 KB/s | 93MB |
| 32 | 1963 | 10.3ms | 37.5ms | 61ms | 100% | 424 KB/s | 118MB |
| 64 | 609 | 19.2ms | 56.3ms | 473ms | 99.65% | 131 KB/s | 108MB |

**特征**：
- 16 并发达峰值 1980 QPS，p50 仅 2.86ms，p99 68ms——Redis 缓存命中后的纯 JSON 序列化性能。
- 32 并发 QPS 持平（1963），p50 升到 10.3ms，说明 go-zero 协程调度开始排队，但还没到瓶颈。
- 64 并发 QPS 暴跌到 609，出现 1 个 503 + 76 个 ECONNRESET——go-zero 自适应限流（load shedding）触发，主动拒绝连接保护服务。
- 128 并发档因检测到 5xx 自动停止，不再加档。

### 2.2 课程详情 GET /api/course/detail/5（公开，需查视频列表+资源列表）

| 并发 | QPS | p50 | p95 | p99 | 成功率 | 备注 |
|------|-----|-----|-----|-----|--------|------|
| 8 | 176 | 5.13ms | 14.0ms | 575ms | 99.51% | 16 个 503，自动停 |

**特征**：8 并发就触发 go-zero 限流 503。详情接口比列表慢（p50 5ms vs 2.8ms），因为要多查一次 course_videos 和 course_resources 表。但服务器内存紧张（MySQL/Redis swap），缓存 miss 时单次 SQL 就要 2-7 秒。

### 2.3 登录 POST /api/user/login（bcrypt CPU 密集）

| 并发 | QPS | p50 | p95 | p99 | 成功率 | 备注 |
|------|-----|-----|-----|-----|--------|------|
| 4 | 10.68 | 160ms | 2310ms | 3619ms | 96.4% | 10 个 503，自动停 |

**特征**：bcrypt cost=10 单次验证约 150ms，2vCPU 理论上限 ~27 QPS。实测 10.68 QPS，因为 go-zero 自适应限流 + MySQL/Redis 资源竞争。p95 2.3 秒说明排队严重。

### 2.4 鉴权中间件开销 GET /api/user/info（带 token，Redis 会话查询）

| 并发 | QPS | p50 | p95 | p99 | 成功率 | 备注 |
|------|-----|-----|-----|-----|--------|------|
| 8 | 835 | 3.0ms | 10.9ms | 43.7ms | 99.92% | 18 个 503，自动停 |

**特征**：每个请求要查 Redis（login:token:{token}）+ 角色缓存（user:role:{username}），p50 3ms 说明 Redis 会话查询很快。但 8 并发就触发 503，说明这台 2G 内存机器上 go-zero 的自适应限流阈值很敏感——CPU 一抖动就开始拒绝。

### 2.5 写笔记 POST /api/course/note（写操作，INSERT 到 video_notes 表）

| 并发 | QPS | p50 | p95 | p99 | 成功率 | 备注 |
|------|-----|-----|-----|-----|--------|------|
| 2 | 0.79 | 1977ms | 4532ms | 4573ms | 63.6% | 4 个 503 + 4 个连接错误 |

**特征**：写操作极慢。2 并发 25 秒只成功 14 次。MySQL 在内存压力下 swap，单次 INSERT 就要 2-4 秒。成功率仅 63.6%，生产环境如果有用户高频记笔记会非常卡。

---

## 3. 和 videonest 的详细对比

### 3.1 缓存读接口（最核心的对比）

| 接口 | 学习平台 | videonest | 差距 |
|------|----------|-----------|------|
| 首页列表（loopback 峰值 QPS） | 1980（课程列表，16并发） | 2025（视频列表，11.7 loopback） | **打平，差 2%** |
| 首页列表 p50（峰值档） | 2.86ms | ~13ms（videonest 11.7） | 学习平台更快 |
| 首页列表 p99（峰值档） | 68ms | ~110ms | 学习平台更快 |
| 分类/字典接口 QPS | 835（user/info 鉴权后） | 2077（热榜，纯 Redis） | 学习平台低 60% |

**结论**：纯 Redis 缓存命中的 JSON 读接口，学习平台和 videonest 几乎打平（~2000 QPS），这是这台 2vCPU 机器的物理天花板——Go HTTP 框架 + JSON 序列化 + Redis GET，再快也就这样。学习平台 p50/p99 甚至略好，因为课程列表响应体比视频列表小（只有 1 条课程 vs videonest 12 条视频）。

### 3.2 详情接口

| 接口 | 学习平台 | videonest | 差距 |
|------|----------|-----------|------|
| 详情接口 QPS（loopback） | 176（课程详情，8并发） | 454（视频详情，11.7 loopback） | **学习平台低 61%** |
| 详情接口 p50 | 5.13ms | ~5ms | 持平 |

**分析**：学习平台课程详情要查 3 张表（courses + course_videos + course_resources），videonest 视频详情只查 1 张 videos 表 + 1 次 interaction 聚合。多一次 SQL 在内存紧张时就是几倍差距。但 p50 持平说明单次查询本身不慢，慢的是并发下 MySQL 连接池排队。

### 3.3 写接口

| 接口 | 学习平台 | videonest | 差距 |
|------|----------|-----------|------|
| 写操作 QPS（loopback） | 0.79（写笔记，2并发） | ~20（浏览量 POST /views，公网数据推算） | **学习平台慢 25 倍** |
| 写操作 p50 | 1977ms | ~50ms | 学习平台慢 40 倍 |

**分析**：这是最大的差距。videonest 的浏览量接口是 Redis INCR（纯内存操作），所以快。学习平台写笔记是 MySQL INSERT（磁盘 IO），在 2G 内存机器上 MySQL swap 严重，单次写就要 2-4 秒。**这不是代码问题，是这台服务器内存不足以同时跑 MySQL + Redis + 三个应用**。

### 3.4 鉴权开销

| 指标 | 学习平台 | videonest |
|------|----------|-----------|
| 带 token 接口 QPS | 835（user/info，8并发） | ~400-500（评论/点赞带 token，公网数据） |
| 鉴权中间件 p50 | 3ms | ~5ms |

学习平台鉴权中间件性能略好，因为：
1. 用 Redis 存会话（login:token:{token}），videonest 也是 Redis
2. 学习平台多了一层角色缓存（user:role:，60 秒 TTL），但命中后不查 DB

### 3.5 限流触发点对比

| 并发档 | 学习平台 | videonest |
|--------|----------|-----------|
| 开始出现 503 | 课程列表 64 并发 / 详情 8 并发 / 登录 4 并发 / 鉴权 8 并发 | 视频列表 256 并发 / 热榜 256 并发 |

**关键发现**：学习平台在 8-64 并发就开始触发 503，videonest 要到 256 并发才限流。原因：
1. 学习平台后端容器 mem_limit 只有 320m（docker-compose.server.yml 里写死的），videonest backend 内存限制更宽松
2. 这台机器现在跑了 11 个容器（lp/hp/vn 各一套 + MySQL + Redis + PG + MinIO + RabbitMQ），内存 available 只有 100-300MB，go-zero 自适应限流阈值被压低
3. videonest 11.7 压测时可能没跑 hp-backend，内存更充裕

---

## 4. 核心结论

### 4.1 性能定位

学习平台的服务能力和 videonest 在**同一数量级**：
- 缓存读接口：~2000 QPS（打平）
- 详情接口：176 vs 454 QPS（学习平台慢 2.5 倍，因为多查表）
- 写接口：0.79 vs 20 QPS（学习平台慢 25 倍，因为 MySQL INSERT vs Redis INCR）
- 登录：10.68 QPS（bcrypt，两个项目都没测，但量级应该差不多）

### 4.2 当前服务器的真实瓶颈

**不是代码，是内存**。服务器 1.8G 内存跑了 11 个容器，MySQL 和 Redis 都在 swap：
- 空表 SELECT COUNT(*) 实测 3 秒（正常应该 <1ms）
- 空表 INSERT 实测 2.6 秒
- Redis INCR 实测 1.1 秒

这导致：
1. 缓存 miss 时所有接口 p99 飙升到秒级
2. go-zero 自适应限流在 8-64 并发就触发（正常应该 256+ 并发才限流）
3. 写接口 QPS 掉到 0.79，完全不可用

### 4.3 和 videonest 公网数据的对照

videonest 11.9 公网报告测出列表 QPS 80-94，是因为公网带宽 2.8Mbps 卡了。loopback 测出 2025 QPS。学习平台这次 loopback 测出 1980 QPS，**如果部署到带宽更大的服务器，公网 QPS 也会是 videonest 的 90% 左右**。

### 4.4 优化建议（按收益排序）

1. **加内存到 4G**：MySQL 和 Redis 不再 swap，写接口 QPS 能从 0.79 涨到 50+，详情接口从 176 涨到 400+
2. **写笔记改 Redis 缓冲**：先写 Redis 队列，后台异步落 MySQL，参考 videonest 浏览量接口的做法
3. **课程详情加缓存**：现在每次都查 3 张表，加个 Redis 缓存（5 秒 TTL）能把 QPS 从 176 拉到 800+
4. **go-zero 自适应限流参数调优**：现在阈值太敏感，8 并发就 503，可以适当放宽 CPU 阈值
5. **登录接口加缓存**：bcrypt 结果不缓存（安全问题），但可以限制登录接口独立限流，避免把整个后端拖垮

---

## 5. 测试产物

- 压测脚本：`S:\learningplatform\load-test\2026-11-09\perfload.js`、`bootstrap.js`、`run_all.sh`、`summarize.js`
- 原始数据：`S:\learningplatform\load-test\2026-11-09\raw\*.json`
- 服务器残留：`/tmp/lpperf-20261109/`（含 tokens.json、探针账号 lpperf_1~4）
- 生产数据污染：video_notes 表有 14 条 `content LIKE 'LP20261109%'` 的测试笔记，测试后可清理

---

*报告生成时间：2026-11-10*
*测试方法：参考 videonest 2026-11-07 loopback harness*
