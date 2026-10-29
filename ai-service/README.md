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
