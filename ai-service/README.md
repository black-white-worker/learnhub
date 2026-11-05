<<<<<<< HEAD
# AI 服务（ai-service）

学习平台的独立 AI 微服务，FastAPI + LangGraph + Chroma + bge-m3。

## 定位
- 只负责：调大模型、Prompt 管理、向量检索、会话管理
- 不承载业务：成绩/批改记录/业务状态都在 Go 的 homework-rpc
- 部署：docker 内网，端口 8000 不对外暴露，外部请求由 Go 主服务转发

## 目录
```
ai-service/
├── app/
│   ├── main.py            # FastAPI 入口，启动时拉起批改消费者
│   ├── api/chat.py        # /api/ai/chat SSE 答疑、/summary、/generate
│   ├── core/config.py     # pydantic-settings 配置
│   ├── graph/rag_graph.py # LangGraph 4 节点图：rewrite→retrieve→prompt→llm
│   ├── rag/
│   │   ├── vector_store.py # 向量库抽象层（Chroma，将来换 Milvus 只改这里）
│   │   ├── embedding.py    # bge-m3 向量化
│   │   └── session.py      # Redis 会话历史，按 user+course 隔离，留最近5轮
│   ├── tasks/grade_worker.py # Redis 队列异步消费作业批改任务
│   └── grpc_server/       # gRPC 骨架（后续练习微服务时打开，端口50051）
├── scripts/ingest.py      # 一次性数据灌入：MySQL→切片500/重叠50→Chroma
├── Dockerfile
└── requirements.txt
```

## 五大功能
1. **作业 AI 批改**（异步）：学生提交→Go 推 Redis 队列→本服务消费→LLM 打分→HTTP 回调 Go
2. **AI 答疑 RAG**（同步 SSE）：/api/ai/chat，4 节点 LangGraph，带引用溯源
3. **AI 学习总结**：/api/ai/summary，提炼3个考点
4. **AI 出题**：/api/ai/generate，老师输入知识点，生成题目+答案，老师审核后入库
5. **会话管理**：Redis 存最近5轮，按 user_id+course_id 隔离

## 启动
```bash
docker-compose up -d ai-service mongo
# 本地开发：
cd ai-service
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000
```

## 数据灌入
```bash
python -m scripts.ingest --course-id 1
```

## 面试话术
- 向量库抽象层：接口隔离，Chroma→Milvus 只改一个文件
- 异步批改：LLM 1~5s 不能阻塞提交接口，Redis MQ 解耦，失败入死信队列
- RAG 引用溯源：答案末尾列命中章节标题，兜底幻觉
- 会话截断：只留最近5轮，防 token 爆炸
- 限流：Redis 计数每日额度，防刷 token
- gRPC 预留：grpc_server 目录已留骨架，后续和 go-zero zrpc 跨语言对接
=======
# ai-service（规划中）

AI 答疑与作业初评服务。当前目录只有环境模板与契约，**尚无实现**；
保留它是为了让下一步的开发有明确的边界，而不是把 LLM 调用塞进主服务。

## 为什么独立

- LLM 调用延迟高、失败率高，不能阻塞主请求；
- 向量库、embedding 模型、prompt 迭代节奏与业务代码完全不同；
- 主服务只需要一个 HTTP 客户端，拆分成本低。

## 契约（草案）

所有接口只在内网暴露，用 `X-Internal-Token` 校验，响应统一 `{code, message, data}`。

### 1. 答疑

```
POST /v1/answer
{
  "classId": 12,
  "courseId": 3,
  "userId": 42,
  "question": "这一章的快速排序为什么最坏是 O(n²)？",
  "topK": 5
}
→ 200
{
  "code": 0,
  "data": {
    "answer": "...",
    "references": [
      { "resourceId": 8, "title": "算法讲义.pdf", "snippet": "..." }
    ],
    "model": "qwen-plus",
    "latencyMs": 1830
  }
}
```

约束：

- 只检索该用户**有权访问**的课程资料（权限由主服务在调用前校验）；
- 引用必须带 `resourceId`，前端可跳转到对应讲义；
- 没有检索到资料时返回"资料中未找到"，不要编造。

### 2. 作业初评

```
POST /v1/grade
{
  "assignmentId": 21,
  "submissionId": 88,
  "content": "学生的文字作答",
  "rubric": "评分要点（来自作业题目）"
}
→ 200 { "code": 0, "data": { "score": 82, "comment": "...", "confidence": 0.78 } }
```

约束：

- 结果只是**建议**，必须由老师确认后才写入成绩册；
- `confidence < 0.6` 时前端提示"建议人工复核"；
- 不把学生答案用于训练。

### 3. 资料入库（异步）

```
POST /v1/embed
{ "courseId": 3, "resourceId": 8, "objectName": "course/3/resources/x.pdf" }
→ 202 { "code": 0, "data": { "jobId": "..." } }
```

- 由主服务在讲义上传成功后调用；
- 幂等：同一 `resourceId` 重复调用不重复计费；
- 失败重试 3 次后落死信，不影响上传主流程。

## 环境变量

见 [`.env.example`](.env.example)：

| 变量 | 说明 |
|---|---|
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | OpenAI 兼容的对话模型 |
| `EMBEDDING_BASE_URL` / `EMBEDDING_MODEL` | 向量化模型 |
| `REDIS_URL` | 任务队列 / 结果缓存 |
| `GO_CALLBACK_BASE` | 回调主服务（写回初评结果） |
| `VECTOR_DB_DIR` | 本地向量库目录（后续可换 pgvector） |

## 落地顺序建议

1. 只做 `POST /v1/answer` + 讲义切片入库（PDF 先跑通）；
2. 加引用与"未找到"兜底，做评测集（20 个真实问题）；
3. 再做作业初评（老师确认后才落库）；
4. 最后接通知与消息中心。
>>>>>>> 048693acea9fa17beb3212e37b5d7e304061d17b
