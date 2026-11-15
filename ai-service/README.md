# AI 服务（ai-service）

学习平台的独立 AI 微服务，FastAPI + LangGraph + Chroma + **MongoDB 知识库** + DashScope（qwen-plus / text-embedding-v3）。

## 定位
- 只负责：调大模型、Prompt 管理、向量检索、会话管理、知识灌入与覆盖度
- 不承载业务：成绩/批改记录/业务状态都在 Go 主服务（`assignment_ai_grades` 等表）
- 部署：docker 内网，端口 8000 不对外暴露，外部请求由 Go 主服务转发
- 进程分工：`ai-service` 只跑 HTTP，`ai-worker` 只消费批改队列（同一镜像不同 command）

**存储分工（这是本轮改造的核心）**：MySQL 是课程元数据的真相源；Redis 守热会话与队列；
**Chroma 只做语义向量检索**；**MongoDB 才是「灌了什么、解析到哪一步、哪里还空着」的事实源**。

## 目录
```
ai-service/
├── app/
│   ├── main.py             # FastAPI 入口 + X-Internal-Token 中间件；/health（存活）/readyz（就绪）
│   ├── api/
│   │   ├── chat.py         # /api/ai/chat SSE 答疑
│   │   ├── generate.py     # /api/ai/generate 出题
│   │   ├── session.py      # /api/ai/session/history、DELETE /api/ai/session
│   │   ├── grade.py        # /api/ai/grade、/api/ai/task/{id}（只给 Go 后端调）
│   │   └── kb.py           # /api/ai/kb/status 知识库覆盖度（只返回聚合数字）
│   ├── questions/          # 出题域（纯函数可单测）
│   │   ├── variety.py      # 随机命题蓝图、去重、选项洗牌、中文/专业度校验
│   │   ├── zhsource.py     # 按知识点检索简体中文权威条目作命题依据
│   │   └── bank.py         # 可选英文通识题库（OpenTDB / The Trivia API）
│   ├── core/config.py      # pydantic-settings：连接/认证/开关全部环境变量化
│   ├── graph/rag_graph.py  # LangGraph 4 节点：rewrite→retrieve(混合召回)→prompt→llm
│   ├── kb/
│   │   ├── store.py        # KnowledgeStore 抽象 + Mongo 实现 + 内存降级实现 + 工厂
│   │   ├── tokens.py       # 中文 bigram / 英文词轻量分词（零第三方依赖）
│   │   └── search.py       # hit 归一化 + RRF 融合（纯函数）
│   ├── rag/
│   │   ├── vector_store.py # 向量库抽象层（Chroma，将来换 Milvus 只改这里）
│   │   ├── embedding.py    # 向量化
│   │   ├── parsers.py      # PDF/DOCX/PPTX/TXT/SRT/VTT 正文解析 + 视频 STT TODO 状态
│   │   ├── source.py       # 本地目录 / MinIO SDK / URL 三路取原文件
│   │   └── session.py      # Redis 会话历史，按 user+course 隔离，留最近5轮
│   └── tasks/              # worker / grade_worker / grade_store（Redis 任务态）
├── scripts/ingest.py       # 知识灌入：课程资料清单 → 拉文件 → 解析 → 切片 → Mongo+Chroma 双写
├── tests/                  # pytest，不打外网、不连真实 Mongo/Redis
├── Dockerfile
├── .dockerignore           # 防止 .env 被 COPY 进镜像
├── .env.example            # 环境变量模板（含上线必配清单）
└── requirements.txt
```

## 四大功能
1. **作业 AI 批改**（异步）：老师点「AI 初评」→ Go 从库里拼载荷 → `POST /api/ai/grade` 带 `taskId` 入队并立刻返回 → `ai-worker` 消费、LLM 打分 → HTTP 回调 Go 写建议分。
2. **AI 答疑 RAG**（同步 SSE）：`/api/ai/chat`，4 节点 LangGraph，混合检索 + 引用溯源。
3. **AI 出题**：`/api/ai/generate`，老师输入知识点，生成题目+答案，老师审核后入库。
4. **会话管理**：Redis 存最近 5 轮，按 user_id+course_id 隔离。

批改结果只写「建议分」，成绩册必须老师落笔；解析失败的作答进 `ai:grade:manual` 并标记 `manual`，不会写 0 分。

## AI 出题：简体中文专业题、随机性与可选通识题库

`POST /api/ai/generate` 的老师端出题已不是「同一句 prompt 调一次模型」。为避免同一知识点
反复生成出同一批题，服务端做了四层随机化（纯函数集中在 `app/questions/variety.py`）：

1. **随机命题蓝图**：每次请求由 `seed` 派生，逐题随机分配互不重复的考查角度（概念辨析 /
   定量计算 / 实验现象 / 易错陷阱 / 情境应用 / 条件变更 / 图像数据 / 对比迁移 / 因果解释 /
   过程排序）、情境、认知层级、干扰项策略，并打散正确答案所在的字母位置；
2. **采样抖动**：`temperature` 0.82~1.0、`top_p` 0.88~0.97 随机取值，prompt 带本次随机编号；
3. **三层去重**：题干指纹 + 字符 bigram Jaccard + SequenceMatcher 相似度（阈值
   `GENERATE_SIMILARITY_THRESHOLD`，默认 0.82），比对进程内最近 60 条题干、前端回传的
   `exclude_stems`（上一批）、以及本批次内已收题目；模型连续给重复题会换蓝图重试
   （`GENERATE_MAX_ATTEMPTS`，默认 2）；
4. **后处理**：选项洗牌并重映射答案字母、结构/内容质量校验（空壳题干、重复选项、
   答案越界、解析缺失等），废题不返回给老师，数量不足自动补齐。

### 默认链路：按知识点出的简体中文专业题

`app/questions/zhsource.py` 会按老师填的知识点（可加学科）检索**免费、免密钥**的
中文维基百科 / 中文维基教科书条目（`variant=zh-cn` 直接返回简体，CC BY-SA 4.0），
把正文切段后按本次 `seed` 随机抽取若干条事实素材注入 prompt，要求模型：

- 题目必须落实素材里的具体定义、公式、条件、机制或数据，并在 `basis` 字段写出依据编号；
- 题干、选项、解析、考查点全部为**简体中文**、使用该学科规范术语与单位；
- 解析不少于 20 字，写出推理依据并说明主要干扰项错在哪；
- 禁止编造素材外的常数/定律/实验数据，禁止空壳题和通识常识题。

之后由 `app/questions/variety.py` 的 `assess_chinese_quality()` 做中文占比、英文整句、
知识点关联、解析充分度体检：英文题库题 / 整句英文题干直接判废并换蓝图补齐，
内容偏弱只标注提示（不误杀公式类选项）。每次换 seed 会换素材段落，题目随机性同步提升。

条目正文进程内缓存 6 小时，一次请求只打一次外网；检索超时（默认 4s）、源不可达或
没有相关条目时 `meta.material_status` 如实记为 `unavailable / no_match`，自动退回
「纯模型中文专业出题」，**不会让生成按钮失败**。素材只作事实依据，不直接当题返回；
返回体 `meta.materials` 保留条目名与链接便于署名。

### 请求字段

不传这些可选字段完全向后兼容：

| 字段 | 说明 |
| --- | --- |
| `seed` | 本次随机种子；前端每次「换一批」生成新值，同 seed 结果可复现 |
| `exclude_stems` | 上一批题干（最多 20 条），服务端据此判重 |
| `source` | `ai`（按知识点出的中文专业题，默认）/ `bank`（英文通识题库，仅课堂热身） |
| `subject` | 学科语境（如「物理学」），影响术语与检索限定；空值由模型按知识点判断 |
| `grounding` | 是否检索中文资料作依据，默认 `true`；关掉即纯模型出题、跳过外网 |
| `bank_category` | OpenTDB 分类 id，仅 `source=bank` 时有效 |

返回体 `meta` 带 `seed / strategy / attempts / deduped / discarded_count / excluded /
subject / grounding / material_status / material_provider / materials / bank_status / localized`；
题目新增 `explanation`（解析）、`focus`（考查点）、`basis`（命题依据）、
`quality_score`（专业度）、`source`（来源）字段。前端会展示本批随机策略、
引用的中文条目链接与每题依据。

顶层 `invalid_count` 只统计**最终展示的题里**不合格的数量（前端据此置灰），
`discarded_count` 是重试过程中被换掉的废题数（用于诊断与本批策略展示，不再误报成
「有 N 道题不合格」）。上一轮废题的具体原因会回灌到下一轮 prompt，重试是有针对性的
修正而非只换随机种子；两轮都不合格时 `meta.hint` 明确说明「不可勾选入库」。

### 阶段进度流（页面不再只有一个转圈遮罩）

同一个 `POST /api/ai/generate`，请求带 `Accept: text/event-stream` 时返回 SSE：

```
event: progress   {"stage":"prepare|materials|compose|verify|done",
                   "status":"running|done|skipped", "detail":"人话说明",
                   "label":"可选阶段名", "elapsed_ms":1234}
event: result     {...与 JSON 接口完全一致的结果...}
event: error      {"message":"..."}
```

阶段由 `_generate()` 在真实节点发出（准备蓝图 → 中文资料检索 → 第 N 轮模型命制 →
质量校验与排重 → 完成），不靠前端定时器伪造百分比；模型生成那 20~60 秒没有业务事件，
服务端每 5 秒补一个 `: keep-alive` 注释心跳防止中间层按空闲回收连接。
不带该 `Accept` 的旧调用仍是一次性 JSON，脚本 / 老前端 / 单测完全兼容。

前端 `streamAiGenerate()` 优先走进度流，**仅在流完全没建立时**回退到普通 JSON 接口；
已经收到阶段事件后中断不会再重跑一次（避免烧两遍额度）。nginx 对
`/api/ai/generate` 关闭了响应缓冲，Go 代理按 `Accept` 走 SSE 链路，无需新增路由。

### 超时分层

中文专业出题要「检索中文资料 → 调大模型 → 可能再换蓝图重试」，实测 25~120 秒才写出
第一个响应头。各层超时必须由外向内逐级放宽，且**只有 SSE 能卡响应头**：

| 层 | 超时 | 说明 |
| --- | --- | --- |
| nginx（前端 / 生产） | 300s（`/api/ai/chat`、`/api/ai/generate` 600s） | `proxy_read_timeout`，两个 AI 流都关 `proxy_buffering` |
| go-zero AI 路由 | 180s | `rest.WithTimeout` |
| Go 反向代理（非 SSE） | 165s | 整体 context 上限，**不设 `ResponseHeaderTimeout`** |
| 前端 axios | 150s | 最先超时，给老师可操作提示 |
| LLM 单次调用 | 60s | `LLM_TIMEOUT`，最多两轮 |

`backend/internal/handler/ai_proxy.go` 曾给所有 `/api/ai/*` 统一设 10s 响应头超时：
SSE 合理（响应头立刻返回），但普通 JSON 接口要处理完才发响应头，于是出题第 10 秒必被
`ErrorHandler` 掐成 503「AI 服务暂时不可用」。现在按 `Accept: text/event-stream` 分流，
SSE 保留 10s 头超时，普通 JSON 不卡头、只卡整体时长；
`TestAIProxy_SlowJSONUpstreamIsNotCutByHeaderTimeout` 用慢上游做防回归。

### 可选：英文通识题库（仅课堂热身）

`app/questions/bank.py` 接了两个实测可用、无需注册/密钥的公开源 ——
Open Trivia DB（主，`api_token.php` 的 session token 防止重复抽题，CC BY-SA 4.0）与
The Trivia API（备用）。它们是**英文通识题**，不按知识点出题，不能替代中文专业题；
只有老师显式把「题目来源」切到「通识热身」才会走这条链路，抽到后由 LLM 批量翻成中文
（只翻文本，答案索引由代码维护，不会翻错）。任何网络错误、超时、限流都会返回
`bank_status=unavailable` 并自动回退到中文专业题出题。

相关环境变量见根 `.env.example`：中文资料 `ZH_MATERIAL_ENABLED / ZH_MATERIAL_TIMEOUT /
ZH_MATERIAL_MAX_DOCS / ZH_MATERIAL_MAX_SNIPPETS / ZH_MATERIAL_WIKIBOOKS_ENABLED /
GENERATE_REQUIRE_CHINESE`，通识题库 `QUESTION_BANK_ENABLED / QUESTION_BANK_INSPIRE /
QUESTION_BANK_LOCALIZE / QUESTION_BANK_TIMEOUT`，以及 `GENERATE_MAX_ATTEMPTS`、
`GENERATE_SIMILARITY_THRESHOLD`。

## 启动
```bash
# 生产（仓库根目录）：批改 worker 和 Redis 都得起来，否则任务一直卡 processing
docker-compose up -d ai-service ai-worker redis backend

# 本地开发：
cd ai-service
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000
python -m app.tasks.worker          # 批改消费者另开一个终端
python -m pytest tests -q           # 253 个用例，不打外网、不连真实 Mongo
```

> **MongoDB 没装/没起也能跑**：`MONGODB_ENABLED=false` 或连接失败时自动降级为
> `InMemoryKnowledgeStore`，答疑主链路不受影响（只是没有词法兜底、数据不持久）。
> 上线把 `MONGODB_STRICT=true`，连不上直接失败，避免「以为灌进去了其实没有」。

## 内部认证
- 所有请求（除 `/health` 与 `/readyz` 两个探针）必须带 `X-Internal-Token`，与 Go 侧 `AI_INTERNAL_TOKEN` 同值；两边都由环境变量注入，源码不留字面量。
- `INTERNAL_AUTH_ENABLED=true` 且密钥留空时所有接口 403 —— 宁可拒绝服务，也不能「忘了配密钥」就退化成不校验。
- `POST /api/ai/grade` **不放进** Go 的浏览器代理路由表：批改烧 LLM 额度、还会写初评记录，入口只能由服务端调。
- `GET /api/ai/kb/status` 已在 Go 代理白名单（`ai_proxy.go`），只暴露 GET，且响应体不含提问原文。

## 知识库架构

| 层 | 职责 | 为什么用它 |
|---|---|---|
| MongoDB `kb_documents` | 每个资源一条，`status=parsed/fallback/failed` + 失败原因 | 文档模型适配异构素材（PDF 页码 / PPT 页码 / 字幕时间戳 / 链接），让「哪些没解析」可见可排期 |
| MongoDB `kb_chunks` | 切片正文 + `keywords` 中文二元组数组 | MongoDB `$text` 对中文近乎不可用；自己切 bigram 存普通数组，`$in` 查询零额外组件 |
| MongoDB GridFS (`kb_files.*`) | PDF/DOCX 原文件 | 解析策略升级后可重放，不必回 MinIO 重拉 |
| MongoDB `kb_ingestions` / `kb_unanswered` | 灌库日志 / 答不上来的问题 | 聚合出覆盖度：先补缺口最大的资料，用数据定位问题而不是拍脑袋 |
| Chroma | 语义向量检索 | 轻量、本地、按课程隔离，已有抽象层可换 Milvus |
| Redis / MySQL / MinIO | 会话与队列 / 元数据 / 原文件 | 各守本职，不被替换 |

**混合检索**：`node_retrieve` 先做 Chroma 向量 Top-K，再取 MongoDB 中文词法 Top-K，用
**RRF**（`Σ 1/(k+rank)`）融合——余弦距离和词法权重不可比，用排名融合最稳。
embedding 服务抖动时自动退化为纯词法；知识库挂了自动退化为纯向量；**任一路径异常都不会让答疑 500**。

**覆盖度闭环在答案产出后统一判断**（只看「检索为空」会漏掉最常见的情况）：
`no_hit`（两路都没结果）、`fallback_only`（只命中名称+链接的占位切片，正文没解析）、
`llm_no_answer`（LLM 明确回「资料中没有相关内容」）都会写 `kb_unanswered`，
由 `/api/ai/kb/status` 聚合出该优先补哪份资料。

**答疑不再「有资料也拒答」**（三个线上故障，都已固化进 `tests/test_rag_retrieve.py`）：

- 检索为空时不调 LLM，直接给「暂无可检索片段 + 可能还在入库」的可读说明，
  不再产出一句空的「资料中没有相关内容【参考资料】」；
- 提示词里不出现中文拒答原文与「【参考资料】」字面量 —— 实测 qwen-plus 会把这两串
  照抄回答案（哪怕上下文完全相关）。拒答改用英文哨兵 `NO_RELEVANT_MATERIAL`，
  流式输出时由 `RefusalFilter` 就地替换成中文说明，学生看不到哨兵；
- 会话历史里滤掉拒答轮次：拒答会自我强化，上一轮的「没答上来」会让模型这一轮继续拒答。

## 数据灌入

**资料从哪来：课程资源接口，不是再查一遍库。** 灌库前先向 Go 要一份清单
`GET /api/internal/course/:courseId/kb-manifest`（`X-Internal-Token`），内容就是课程
详情页那份讲义列表 + 课程视频列表（私有桶下已换好预签名 URL）。
这样「页面上能看到的讲义」与「灌进知识库的讲义」永远是同一份；只有 Go 暂时不可达时，
才退回直读 MySQL `course_resources`（`app/kb/manifest.py` 里 `resource_rows` 的
`None`=没取到、`[]`=确实没有，两者不能混）。

**正常使用时不需要手动跑脚本**：老师上传讲义成功后，Go 侧会异步调用
`POST /api/ai/kb/ingest`（带 `X-Internal-Token`），ai-service 在后台守护线程里
解析 → 切片 → MongoDB + Chroma 双写；删除讲义时调用 `POST /api/ai/kb/purge`
清掉切片与向量。同一门课串行灌库（并发会让「先删后写」互相踩踏），
排队期间重复提交返回 `accepted=false`，Go 只记日志不重试。

**课程视频**：正文转写（ffmpeg + ASR）仍未实现，所以视频不切成正文切片；
答疑检索全空时会把「课程视频清单」（标题）当最后一档上下文，问题只到标题粒度，
提示词里明确要求模型说明「正文未转写、不要编造讲解细节」。要让视频真正可检索，
下一步接 ASR 或要求老师上传配套 SRT/VTT 字幕（字幕解析已支持）。

手动通道仍保留，用于回填历史课程 / 排查：

```bash
python -m scripts.ingest --course-id 1
python -m scripts.ingest --course-id 1 --dry-run   # 完全只读：不连 Mongo、不碰 Chroma、不调 embedding

# 本地离线验证：把 object_name 映射到容器内目录（注意用相对路径）
docker exec -e INGEST_LOCAL_FILE_ROOT=data/ingest_local learningplatform-ai \
  python -m scripts.ingest --course-id 4
```

> **Embedding 必须分批**：DashScope `text-embedding-v3` 单次请求最多 10 条 input。
> 一份 1MB 的 docx 切出 87~209 个片段，一把梭会 `400 batch size is invalid`，
> 而 ingest 把异常记成「该资源灌入失败」→ 知识库一直是空的 → 答疑只会回
> 「资料中没有相关内容」。`EMBEDDING_BATCH_SIZE`（默认 10）会自动分批，顺序严格对齐。

**这条路已经打通**（旧版只灌 `name+url`，PDF/视频正文是 TODO，答疑经常答「资料中未找到」）：

- MySQL 连接全部走 `settings`，源码不再有 `localhost/root/root`；`SELECT` 带上 `object_name/type`；
- 按类型真解析：`PDF`(pypdf，带页码)、`DOCX`/`PPTX`(标准库 zip+XML，带段落/幻灯片号)、`TXT`、
  `SRT`/`WebVTT` 字幕（按时间窗口合并，带 `timestamp/start/end/cue_count`，可回溯到视频时间点）；
- 切片 metadata 补上 `resource_id`，`ChromaStore.delete_by_resource` 从此可用，支持增量重灌；
- Mongo `replace_chunks` 与 Chroma `delete→upsert` 都是先删后写，内容哈希幂等，重跑不产生重复切片；
- 每个资源写 `kb_documents.status`，`GET /api/ai/kb/status?course_id=1` 可查覆盖度。

**已实测跑通（本地课程 4，用一份真实 DOCX 讲义）**：`parsed=1 / fallback=0 / chunks=5 / files=1`，
MongoDB 与 Chroma 各 5 条切片、原文件进 GridFS；连灌两次仍各 5 条（幂等）。
随后 admin 登录 → SSE 问「讲义里提到的专属校验关键词是什么？」→ 回答「蓝犀牛协议」并带
【参考资料】，不再回「资料中未找到」，`unanswered_queries` 不增长。
**这才是「知识灌入走通了」的实证，而不是只有单测。**
讲义是本地演示数据，放在容器 volume 的 `data/ingest_local/` 下，不改 MySQL 与 MinIO 原始数据；
删掉它再灌一次即可回到 `fallback` 状态。

**仍如实标注为未完成**（面试务必主动说明）：

1. **视频本体转写仍是 TODO**：`type=mp4` 资源本身不做 ASR，`status=fallback`，
   `reason` 明写「ffmpeg 抽音频 + STT 尚未实现」。`INGEST_PARSE_VIDEO` 默认 false，打开也仍返回 unsupported。
   **但配套 `.srt`/`.vtt` 字幕已支持**：单独作为字幕资源上传即可解析正文（`status=parsed`），
   这是不等 ASR、先把视频内容灌进知识库的务实折中。
2. **老式 `.doc` / `.ppt` 二进制格式**不支持，建议先转 PDF/DOCX；**扫描件 PDF** 无文本层时需 OCR（TODO）。
3. **双写没有分布式事务**：Mongo 与 Chroma 之间靠「先删后写 + 内容哈希」保证最终一致，
   单资源异常会被隔离并标 `failed`，重跑该课程即可修复。
4. **内存降级只在单进程内有效**，跨进程不共享；生产必须开 MongoDB。
5. `GRPC_PORT` 仍只是预留：当前 Go↔Python 走 HTTP 转发，gRPC 未实现。

## 环境变量
见 [`.env.example`](.env.example)（含完整的本地/上线两段说明）。要点：
`LLM_API_KEY`、`EMBEDDING_*`、`REDIS_URL`、`MONGODB_URL`、`MYSQL_*`、`MINIO_*`、
`GO_CALLBACK_BASE`、`INTERNAL_TOKEN`、`DAILY_TOKEN_LIMIT`、`MONGODB_STRICT`、`RETRIEVAL_*`、
`INGEST_PARSE_SUBTITLE`、`INGEST_LOCAL_FILE_ROOT`。

> Windows Git Bash 下 `docker exec -e INGEST_LOCAL_FILE_ROOT=/app/data/...` 会被 MSYS
> 自动转换成本机路径；容器内请用相对路径（`data/ingest_local`）。

### 上线服务器必配清单
1. **MongoDB**：`MONGODB_STRICT=true`，副本集 URL + 用户名/密码/authSource + `MONGODB_TLS=true` +
   连接池与超时；`mongo` 服务启用 `MONGO_INITDB_ROOT_*` 且**不映射端口到宿主机**。
2. **MySQL**：给 ingest 一个只读账号（只需 `SELECT course_resources`）。
3. **MinIO**：内网 endpoint `minio:9000` + 独立 AK/SK（私有桶按 `object_name` 拉取）。
4. **密钥**：`INTERNAL_TOKEN`（与 Go 同值）、`LLM_API_KEY`、`REDIS_URL` 全部由密钥管理注入；
   `.dockerignore` 已确保 `.env` 不会被 `COPY . .` 打进镜像。
5. **探针**：`/health` 只探进程存活（不因 Mongo 抖动重启容器），`/readyz` 暴露
   `knowledge_base.backend / degraded` 供就绪判断与告警。
6. **上线流程**：重建镜像 → 执行一次 `ingest --course-id <id>` → 查
   `GET /api/ai/kb/status?course_id=<id>` 核对 `parsed_resources / fallback_resources / unanswered_queries`。

## 测试
`python -m pytest tests -q`（253 个用例，全部 mock 外部依赖）：

| 测试类 | 覆盖功能 |
|---|---|
| `test_config.py` | 连接环境变量化、生产 Profile、密钥不进 repr |
| `test_kb_tokens.py` / `test_kb_search.py` | 中文分词/词法打分、RRF 融合与去重 |
| `test_kb_store.py` | Mongo（fake collection）与内存实现的等价 CRUD、索引、幂等、故障降级、严格模式 |
| `test_parsers.py` / `test_source.py` | PDF/DOCX/PPTX/TXT 解析、视频 TODO 诚实状态、路径穿越防护、协议白名单 |
| `test_subtitles.py` | SRT/WebVTT 时间轴、NOTE/STYLE 与内联标签过滤、30 秒窗口合并、开关关闭、无字节降级 |
| `test_vector_store.py` | 切片 ID 唯一性与稳定性、显式 embeddings、按资源删除、查询结构 |
| `test_ingest.py` | 连接配置化、按类型解析、`resource_id`、双写幂等、失败隔离、dry-run 只读 |
| `test_rag_retrieve.py` | 混合召回、故障降级不 500、未命中日志闭环、下游节点兼容 |
| `test_kb_api.py` | 覆盖度接口鉴权、不泄露提问原文、健康探针分离 |

Go 侧 `backend/internal/handler/ai_proxy_test.go` 覆盖新路由白名单与只读约束。

## 面试话术
- **MongoDB 不是死配置了**：原先只在 compose/config/requirements 里挂着却没人 import。
  我让它承担 RAG 知识库的**文档主存储 + 解析状态机 + 审计 + GridFS 原文件**，
  正好补上「灌了什么全靠猜」的盲区；Mongo 挂了自动降级内存，服务不崩。
- **答疑答不上来的根因不是向量库不行**：旧版只切资源名+链接、PDF/视频正文没解析，
  纯向量召回在稀疏库下必然漏。修法是打通 ingest 按类型真解析 +
  Chroma 语义召回叠加 Mongo 中文词法兜底（RRF 融合）+ 未命中日志做覆盖度闭环。
- **别只说「我打通了」，要能拿出实证**：本地课程 4 用真实 DOCX 讲义实测
  `parsed=1 / chunks=5`，Mongo 与 Chroma 双写各 5 条、连灌两次不翻倍，
  SSE 答疑能答出讲义里的专属关键词并附参考资料；改造前同一门课是
  `fallback=1`、答疑只能回「资料中未找到」。
- **视频 STT 我没做完，但不装作做完**：本体转写仍是 TODO，`kb_documents.status` 让缺口可见；
  同时补了配套 SRT/VTT 字幕解析，有字幕的资源不用等 ASR 也能灌正文，切片带时间戳可回溯。
- **中文词法为什么不用 `$text`**：MongoDB 自带 text index 按空格切词，中文没有分词边界；
  引入 ES/jieba 又会给「本地能跑」加依赖，所以用标准库切中文 bigram 存普通数组，`$in` 查询即可。
- **pymongo 是同步客户端**：在线检索用 `asyncio.to_thread` 挪出事件循环，
  不引入第二个 MongoDB 驱动；liveness 与 readiness 分开，依赖抖动不会把容器判死重启。
- 其余：向量库抽象层（Chroma→Milvus 只改一个文件）、异步批改（Redis MQ 解耦、taskId 由调用方给定）、
  引用溯源兜底幻觉、会话截断防 token 爆炸、Redis 每日限流、worker 失败转人工而非给 0 分。
