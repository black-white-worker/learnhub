# B 端性能规范：并发与查询

> 适用对象：本仓库后端（go-zero + 手写 SQL）与后续拆出的微服务。
> 目标：给"班级 / 课程 / 作业 / 考试 / 成绩册"这类 **B 端**场景定一套可执行的取舍标准，
> 避免出现"P95 看起来还行，但连接池已经被打满"的假优化。
>
> 配套：压测脚本 `load-test/`，历史报告 `load-test-report/REPORT.md`（小数据集短压，仅看趋势）。

---

## 0. 结论先行：这次改了什么

| 位置 | 原来的问题 | 做法 | 验证 / 收益 |
|---|---|---|---|
| `class/logic/listannouncementslogic.go` | 每条公告一次 SQL（N+1），再用 8 路 `errgroup` 掩盖 | 一条 `GROUP BY` 批量聚合已读数 + 我是否已读 | 已读查询 **20 次 → 1 次**（整页固定 2 条 SQL）；回传行数 O(N×已读人数) → O(N) |
| `model/grade.go` `ListGradebookRaw` | 4 个相关子查询，对每个学生各执行一次 | 先按学生预聚合，再 `LEFT JOIN`；视频段用 `STRAIGHT_JOIN` 固定驱动顺序 | 120 人 / 4560 条成绩：**268~296ms → 59~107ms**（EXPLAIN ANALYZE） |
| `model/video_progress.go` `SaveVideoProgress` | 读 → 写 → 再读（3 次往返），而这是全站最高频写路径 | 纯函数算出结果，写完直接返回 | **3 → 2 次往返**；累加/封顶/完成标记有单测 |
| `class/logic/classdetaillogic.go` | 班级详情 4 段查询串行 | 4 路**有界**并发 | 取数延迟 4×RTT → ≈1×RTT |
| `exam/logic/autosubmit.go` | 每份答卷回查草稿、题目、成绩项；串行写库 | 草稿随扫描带出；题目按考试批量取；成绩项每场只 ensure 一次；4 路有界并发 + 条件更新 | 每份答卷 **4~6 → 2 次**查询（题目/成绩项按考试摊销）；收卷扫描 960 行 → 48 行 |
| `class/common.go` `CurrentUser` | 每个请求回源 `users` 查 id | 登录会话携带 `userId`，老会话回落 | **每请求省 1 次查询**（班级模块所有接口受益） |
| `deploy/sql/migration/V22__*.sql` | 列表/聚合缺索引或排序缺覆盖 | 按真实 SQL 补 7 个复合索引（幂等迁移） | 见 §3.7 与各条 EXPLAIN |

> 数据口径：本机原生 MySQL 9.6，单实例，合成数据（120 名学生 / 4560 条成绩 / 3840 条签到 /
> 3456 条进度）。耗时来自 `EXPLAIN ANALYZE` 的单次执行（重复 3 次取范围），
> **不是容量规划依据**，只用于判断改写有没有数量级退化。

---

## 1. 优化顺序：先修查询，再谈并发

B 端接口的瓶颈通常不是"CPU 不够"，而是**往返次数**和**重复计算**。所以固定顺序是：

```
1. 这条 SQL 该不该存在？   → 合并（JOIN / 批量 IN / GROUP BY）
2. 它走索引了吗？          → EXPLAIN：访问类型、扫描行数、是否 filesort
3. 还能并行哪些独立查询？  → 有界并发（收益是 RTT，不是吞吐）
4. 读多写少值得缓存吗？    → 缓存 + 明确失效策略
5. 写入高峰能削峰吗？      → 异步/队列（先想清楚一致性代价）
```

**反例（本次修掉的）**：公告列表的已读状态是典型的 N+1。
旧实现给每条公告开一个 goroutine 去查 `announcement_reads`，用 8 路信号量"限流"。
延迟确实从 N×RTT 降到了约 1×RTT，但**总查询数、总传输字节、连接池占用都没有变**，
高峰期只是把"慢"换成了"抖动"。正确做法是让它变成 1 条 SQL。

---

## 2. 六条硬规则

### R1 循环里禁止数据库调用（N+1）

- 优先：`JOIN`、`WHERE id IN (...)` 批量查、`GROUP BY` 聚合。
- 一次批量接口只允许一条动态 `IN`，占位符必须按切片长度生成（见 `AnnouncementReadStats`）。
- 例外：集合大小有硬上限（≤ 8）且已在注释里说明为什么不能批量。没有例外就别写循环查询。

### R2 并发必须有界、可取消

统一入口：`internal/platform/concurrency.ForEach(ctx, n, limit, fn)`。

- `limit <= 0` 一律按 **1** 处理，绝不退化成无界并发；
- 任一任务出错会取消其余任务，并返回第一个错误；
- panic 转成 `*PanicError`（带堆栈）返回，后台任务不会因为一个坏分支打崩进程；
- 后台任务再加一层总超时（自动收卷是 5 分钟），避免永久挂住。

```go
// 典型用法：4 段互不依赖的取数
err := concurrency.ForEach(ctx, 4, 4, func(ctx context.Context, i int) error {
    switch i {
    case 0: /* ... */
    }
    return nil
})
```

**并发度怎么定**：`min(独立任务数, 4~8)`，并且要明显小于连接池上限（go-zero 默认 64）。
并发度不是越大越好——超过 DB 能并行执行的核数之后，只是把排队从 Go 挪到 MySQL。

### R3 索引与 SQL 成对出现

- 只给**已经存在的高频 SQL** 建索引，不为"以后可能会查"建索引；
- 复合索引顺序：`WHERE` 等值列 → `ORDER BY` 列 → 覆盖列；
- 分页列表（`WHERE class_id=? ORDER BY created_at DESC LIMIT ?`）必须有对应的
  `(class_id, created_at)`，否则先取全量再 filesort；
- 迁移必须幂等（V21/V22 的 `information_schema` + `PREPARE` 写法），线上手工热修过也不会中断；
- 高写入表加索引要算写入成本：`video_progress` 的索引只有在 SQL 真的按 `video_id` 驱动时才值得。

### R4 写路径控制往返次数

- 读-改-写最多 **2 次往返**：先读旧值 → 计算 → 一次 upsert；不要写后再读一遍"确认"。
- 并发语义交给数据库，不要靠 Go 里的 `if`：
  - 幂等写入：`INSERT ... ON DUPLICATE KEY UPDATE`；
  - 只允许一次的状态迁移：`UPDATE ... WHERE status='started'`，按 `RowsAffected` 判断；
  - 单调字段：`GREATEST(col, VALUES(col))`，乱序/重复上报不会把进度写小。
- 写失败要区分"辅助数据"和"关键数据"：视频进度、成绩册写失败只记日志，不能阻断观看/交卷。

### R5 身份与热点配置进会话/缓存，但必须有失效路径

- 登录会话（Redis）直接携带 `userId`，业务接口不再每请求查 `users`；老会话 `id=0` 自动回落，
  滚动发布不要求用户重新登录。
- 角色这类"会变但变得少"的字段用 60 秒 Redis 缓存 + 主动失效（`InvalidateRoleCache`）。
- 列表缓存用**版本号 key**（`course:list:ver`）而不是 `SCAN + DEL`：O(1)、不阻塞 Redis、
  新建课程后立即生效。

### R6 每次优化都要能验证

至少满足两项：

1. `EXPLAIN` / `EXPLAIN ANALYZE` 前后对比（扫描行数、是否 filesort、总耗时）；
2. 结果集对照（旧 SQL 与新 SQL 输出逐行一致）；
3. 单测/集成测试（纯函数用例、批量接口与明细接口交叉验证）。

数据量必须写清楚。没有数据量的"快了 10 倍"没有意义。

---

## 3. 逐项说明

### 3.1 公告已读：N+1 不该用并发掩盖

旧实现（每条公告一次）：

```sql
SELECT user_id FROM announcement_reads WHERE announcement_id = ?;
```

Go 侧再遍历 id 列表求 `readByMe`、`readCount`。一页 20 条公告 = 20 次往返，
并且把每条的**完整已读名单**拉回应用层（120 人 × 20 条 = 2400 行）只为算两个数字。

现在（一页一次）：

```sql
SELECT announcement_id,
       COUNT(*) AS read_count,
       MAX(CASE WHEN user_id = ? THEN 1 ELSE 0 END) AS read_by_me
  FROM announcement_reads
 WHERE announcement_id IN (?, ?, ...)
 GROUP BY announcement_id;
```

走 `uk_ann_user(announcement_id, user_id)` 的 IN 范围扫描；返回行数 = 公告条数。
聚合失败时降级为"未读/0 人"并记日志——已读标记是辅助信息，不该把公告列表打成 500。

### 3.2 成绩册：相关子查询 → 预聚合

旧结构（示意）：

```sql
SELECT ..., 
  (SELECT COUNT(*) FROM signin_records ... WHERE cs.class_id = cm.class_id AND r.user_id = cm.user_id),
  (SELECT COUNT(*) FROM video_progress ... WHERE cc.class_id = cm.class_id AND vp.user_id = cm.user_id AND vp.completed = 1)
FROM class_members cm ...
```

两个相关子查询对 `class_members` 的**每一行**各执行一次，再叠加 `grade_scores × grade_items`
的宽连接。121 个学生就是 242 次子查询。

新结构：每类统计先按学生聚合成一张 ≤ 班级人数的临时结果，再 `LEFT JOIN` 回学生列表。
口径完全不变（作业/考试仍按"有成绩的项"累加满分），只是执行次数从 O(学生数×子查询) 变成 O(1) 次扫描。

视频段还有一个坑：优化器默认会**全表扫 `video_progress`**（所有班级的进度都扫），
因为 `GROUP BY user_id` 看起来更适合按用户索引。改成 `STRAIGHT_JOIN` 固定
"班级 → 课程视频 → 进度"的驱动顺序，并配 `idx_video_completed(video_id, completed, user_id)`
覆盖索引后，该子查询从 **78ms 降到 5.7ms**：

```sql
FROM class_courses cc
STRAIGHT_JOIN course_videos cv ON cv.course_id = cc.course_id
STRAIGHT_JOIN video_progress vp ON vp.video_id = cv.id AND vp.completed = 1
WHERE cc.class_id = ?
GROUP BY vp.user_id
```

> 经验：**索引只是"可用路径"，连接顺序决定"会不会走"**。改完 SQL 一定看 EXPLAIN，
> 不要假设"建了索引就会用"。

正确性验证：
- 旧 SQL / 新 SQL 各导出 121 行 TSV，`Compare-Object` 完全一致；
- `model/query_integration_test.go` 覆盖"没有成绩的类别为 0""签到/视频计数""聚合 vs 明细交叉验证"。

### 3.3 视频进度：最高频写路径的 3 → 2

播放器每 15 秒上报一次，100 人在看就是约 6.7 QPS 的纯写；旧实现是
`SELECT 旧进度` → `INSERT ... ON DUPLICATE KEY UPDATE` → `SELECT 新进度`。

现在：读一次旧值，纯函数 `NextVideoProgress` 算出"累加、封顶、完成标记"，
一次 upsert 后直接返回计算结果。规则：

- 观看时长累加，且不超过已知总时长（时长未知不封顶）；
- 总时长取历史与本次的较大值（播放器偶尔上报 0）；
- 完成标记只增不减。

并发注意：多标签页同时上报时，`GREATEST` 保证库里的值不回退；接口返回的是**本次请求算出的值**，
可能略落后于另一个标签页刚写入的值，下一次上报即自动对齐（进度条场景可接受）。
这是"少一次往返"换来的明确取舍，写在了函数注释里。

### 3.4 班级详情：有界并发

一次要取：班级、成员、绑定课程、邀请码。四段互不依赖，串行是 4×RTT。
改成 4 路并发后约 1×RTT，且并发度封顶 4；成员/课程/邀请码查失败仍按旧行为降级为空，
只有班级本体查失败才 404。

### 3.5 自动收卷：后台任务的三条纪律

后台任务最容易被忽视，因为它不在用户请求路径上，但一次全表扫 + 逐份回查足以拖垮整个库。

1. **只扫该扫的**：`WHERE status='started'` 配 `idx_status_started(status, started_at)`，
   实测扫描行数 960 → 48（生产中"进行中"永远是少数）。
2. **不按份回查**：草稿 `answers` 随扫描 SQL 一起带出；题目用 `IN` 一次取回按考试分组；
   成绩项每场考试只 `EnsureGradeItem` 一次，而不是每个学生一次。
3. **有界并发 + 条件更新**：写库 4 路并发，整轮 5 分钟超时；
   `ExpireSubmission` 带 `WHERE status='started'`，学生恰好同时交卷时只有一个成功，
   失败的那条**不会**再用草稿覆盖真实答卷。

单份答卷的查询数从 4 次（草稿 / 题目 / 收卷 / 成绩项）降到 2 次（收卷 / 成绩）。

### 3.6 会话携带 userId

`SessionUser` 增加 `Id`，登录时写入。`class.CurrentUser` 优先用会话里的 id；
老会话（无该字段）反序列化为 0，回落到 `GetIdByUsername`。收益是所有班级模块接口
每请求少一次 `SELECT id FROM users WHERE username=?`。

### 3.7 索引清单（V22）

| 表 | 索引 | 服务的 SQL | 原因 |
|---|---|---|---|
| `class_invitations` | `(class_id)` | 取当前邀请码 / 邀请列表 | 原来只有 `uk_code`，按班查要全表扫 |
| `exam_submissions` | `(status, started_at)` | 自动收卷扫描 `status='started'` | 只扫进行中的答卷（960 → 48 行） |
| `video_progress` | `(video_id, completed, user_id)` | 成绩册按班统计视频完成数 | 覆盖索引；配合 `STRAIGHT_JOIN` |
| `assignments` | `(class_id, created_at)` | 作业分页 `ORDER BY created_at DESC` | 消除 filesort（EXPLAIN 由 `Sort` 变 `reverse index scan`） |
| `exams` | `(class_id, start_at)` | 考试分页 `ORDER BY start_at DESC` | 同上 |
| `announcements` | `(class_id, is_top, created_at)` | 公告分页 `ORDER BY is_top DESC, created_at DESC` | 三级索引可整体倒序扫描 |
| `notifications` | `(user_id, id)` | 消息中心 `ORDER BY id DESC LIMIT 50` | 现有索引无法满足 id 倒序，量大时 filesort |

> 诚实说明：`notifications` 在本次 100 条/人的数据量下优化器仍选 `idx_user_read`
> （100 行排序很便宜）。保留该索引是为了消息量到万级时仍有 `ORDER BY id` 的可用路径，
> 可以用 `FORCE INDEX (idx_user_id)` 验证。索引需要跟着真实数据增长复核，不是一劳永逸。

---

## 4. 参考的开源项目与资料

这些做法不是自创的，主要参考：

- **go-zero**（本项目框架）：`sqlx` 连接池、`redis` 缓存、`singleflight` 合并回源。
  <https://go-zero.dev>
- **golang.org/x/sync/errgroup** 的 `SetLimit`：有界并发的标准实现（本仓库 `concurrency` 包
  在它之上加了"limit<=0 视为 1"和 panic 兜底）。
  <https://pkg.go.dev/golang.org/x/sync/errgroup>
- **Canvas LMS**（成绩册/公告的业界参照）：公告已读状态用预加载/聚合而不是逐条查；
  Gradebook 采用"提交时落分、查询时聚合"；SpeedGrader 用一次队列接口替代逐份往返。
  <https://github.com/instructure/canvas-lms>
- **Open edX**：学习进度按 `(user, video)` 记录累计观看，而不是"播放头位置"。
  <https://github.com/openedx/edx-platform>
- **阿里巴巴 Java 开发手册（p3c）**：ORM/SQL 规约，包括"禁止在循环体内查询数据库"、
  "分页必须带排序且排序字段有索引"。
  <https://github.com/alibaba/p3c>
- **MySQL 8.4 Reference Manual**：`EXPLAIN ANALYZE`、覆盖索引、`STRAIGHT_JOIN` 语义。
  <https://dev.mysql.com/doc/refman/8.4/en/explain.html>
- **《High Performance MySQL》**（Baron Schwartz 等）：索引设计与"先测量再优化"的方法论。

---

## 5. 如何复现验证

临时库（**不要指向业务库**）：

```bash
mysql -e "DROP DATABASE IF EXISTS lp_perf; CREATE DATABASE lp_perf CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
for f in backend/deploy/sql/migration/V*.sql; do mysql lp_perf < "$f"; done
mysql lp_perf < scripts/perf/seed_gradebook_bench.sql
```

然后：

```sql
-- 把 model/grade.go 里 ListGradebookRaw 的 SQL 贴进来（classId 换成 9001）
EXPLAIN ANALYZE SELECT ...;

-- 自动收卷扫描
EXPLAIN SELECT id FROM exam_submissions WHERE status='started';
```

自动化回归：

```bash
cd backend
TEST_MYSQL_DSN='root:...@tcp(127.0.0.1:3306)/lp_perf?charset=utf8mb4&parseTime=true&loc=Local' \
  go test ./internal/model -run TestQueryOptimizations -v
go test ./...   # 无依赖时集成用例自动 skip
```

---

## 6. 还没做，以及"什么时候值得做"

按触发条件排，不做无依据的优化：

| 方向 | 现状 | 触发条件 / 建议 |
|---|---|---|
| 请求级超时与 ctx 透传 | 模型层仍统一用 `context.Background()`，请求取消传不到 SQL | 出现"DB 抖动 → goroutine 堆积"时优先做；需要统一改 model 方法签名，工作量大但收益确定 |
| 连接池参数 | go-zero 默认 64/64/1m，未按实例数调整 | 多实例部署或 `max_connections` 紧张时，按 `max_connections / 实例数` 调整，并在 DSN 加 `timeout/readTimeout/writeTimeout` |
| 公告列表缓存 | 每次 2 条 SQL，已很轻 | 公告读 QPS 明显上来、且写频率低时，加版本号 key 的 30~60s 缓存 |
| 成绩册物化 | 视频聚合 5.7ms/班 | 学生数 × 视频数到万级、或成绩册被高频轮询时，改为"进度变更写汇总表" |
| 视频进度异步批量写 | 2 次往返 | 同时在线观看人数再上一个量级时，Redis 缓冲 + 定时批量落库（要接受进程崩溃丢最后几秒） |
| 考试交卷削峰 | 同步写答卷 + 成绩册 | 大考集中交卷导致写入尖峰时，用队列削峰；必须做幂等（`WHERE status='started'` 已经是基础） |
| 读写分离 / 多级缓存 | 单库 | 读 QPS 成为瓶颈、且能接受主从延迟时再拆 |

---

## 7. 上线检查清单

- [ ] 迁移 `V22` 在预发库执行通过，且**重复执行**不报错（幂等）。
- [ ] 滚动发布：老会话（`id=0`）能正常回落，没有被踢下线；观察 401/500 比例。
- [ ] 发布后 48 小时对比：接口 P95/P99、慢查询条数、连接池 in-use、MySQL CPU。
- [ ] 回滚预案：索引可 `DROP INDEX`；SQL 改动可回滚到旧查询（数据无结构变更，风险低）。
- [ ] 任何新加的"并发"都回答了三个问题：上限多少？失败怎么取消？连接池够不够？
