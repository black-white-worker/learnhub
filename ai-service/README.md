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
