"""主观题批改入口（/api/ai/grade、/api/ai/task/{id}）测试。

这两个接口只允许 Go 后端调用（不在 Go 的浏览器代理路由表里），
测试覆盖：
1. 身份头缺失 / 角色不是老师 → 拒绝
2. 载荷校验：没有题、题数超限、taskId 非法
3. 入队写进的是 settings.grade_queue，且消息带 taskId 与 retry
4. 调用方给的 taskId 原样复用（Go 先落库再入队，靠它对齐两边状态）
5. 任务状态查询：命中 / 过期 / 缺用户头

Redis 与任务存储全部 mock，不依赖真实服务。
"""
import json
from unittest.mock import AsyncMock, MagicMock, patch

from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)

TEACHER_HEADERS = {"X-User-Id": "1", "X-Role": "teacher"}

ONE_ITEM = {
    "questionId": 11,
    "stem": "简述 RAG 的检索流程",
    "studentAnswer": "先切块再做向量检索",
    "standardAnswer": "切块-向量化-召回-重排",
    "maxScore": 20,
}


def make_body(n_items: int = 1, **extra) -> dict:
    items = [{**ONE_ITEM, "questionId": 10 + i} for i in range(n_items)]
    return {"submitId": 99, "assignmentId": 7, "courseId": 3, "studentId": 55, "items": items, **extra}


def fake_store() -> tuple[MagicMock, dict]:
    """返回 (patched grade_store 模块, 内存任务表)。"""
    table: dict = {}

    async def create_task(task_id, payload):
        table[task_id] = {"taskId": task_id, "status": "pending", "itemCount": len(payload.get("items") or []), "result": None, "error": ""}
        return table[task_id]

    async def get_task(task_id):
        return table.get(task_id)

    mod = MagicMock()
    mod.create_task = AsyncMock(side_effect=create_task)
    mod.get_task = AsyncMock(side_effect=get_task)
    mod.STATUS_PENDING = "pending"
    return mod, table


# ---- 身份 ----

def test_grade_requires_user_id():
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=make_body(), headers={"X-Role": "teacher"})
    assert r.status_code == 401


def test_grade_requires_teacher_role():
    """Go 已经鉴过一次，这里再拦一遍：内部令牌泄漏也不能给学生刷分。"""
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=make_body(), headers={"X-User-Id": "9", "X-Role": "student"})
    assert r.status_code == 403
    assert "教师" in r.json()["detail"] or "管理员" in r.json()["detail"]


def test_grade_admin_allowed():
    store, _ = fake_store()
    redis = MagicMock(lpush=AsyncMock())
    with patch("app.api.grade.grade_store", store), patch("app.api.grade._r", redis):
        r = client.post("/api/ai/grade", json=make_body(), headers={"X-User-Id": "9", "X-Role": "admin"})
    assert r.status_code == 200


def test_grade_rejects_invalid_user_id():
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=make_body(), headers={"X-User-Id": "abc", "X-Role": "teacher"})
    assert r.status_code == 400


# ---- 载荷校验 ----

def test_grade_rejects_empty_items():
    body = {"submitId": 1, "items": []}
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=body, headers=TEACHER_HEADERS)
    assert r.status_code == 400


def test_grade_rejects_too_many_items():
    body = make_body(31)
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=body, headers=TEACHER_HEADERS)
    assert r.status_code == 400
    assert "30" in r.json()["detail"]


def test_grade_rejects_submit_id_zero():
    body = {**make_body(), "submitId": 0}
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=body, headers=TEACHER_HEADERS)
    assert r.status_code == 422


def test_grade_rejects_over_max_score():
    body = make_body()
    body["items"][0]["maxScore"] = 500
    with patch("app.api.grade.grade_store", fake_store()[0]), patch("app.api.grade._r", MagicMock(lpush=AsyncMock())):
        r = client.post("/api/ai/grade", json=body, headers=TEACHER_HEADERS)
    assert r.status_code == 422


# ---- 正常入队 ----

def test_grade_enqueues_with_task_id_and_retry():
    store, table = fake_store()
    redis = MagicMock(lpush=AsyncMock())
    with patch("app.api.grade.grade_store", store), patch("app.api.grade._r", redis):
        r = client.post("/api/ai/grade", json=make_body(2), headers=TEACHER_HEADERS)
    assert r.status_code == 200
    data = r.json()
    task_id = data["taskId"]
    assert len(task_id) == 32 and data["status"] == "pending" and data["itemCount"] == 2
    assert data["submitId"] == 99

    # 队列里那条消息必须自带 taskId + retry，worker 靠它回写状态
    queue_name, raw = redis.lpush.call_args.args
    assert queue_name == "ai:grade:queue"
    job = json.loads(raw)
    assert job["taskId"] == task_id
    assert job["retry"] == 0
    assert len(job["items"]) == 2
    assert job["items"][0]["questionId"] == 10

    # 任务表也建了同 id 的记录，前端轮询 GET /task/{id} 才查得到
    assert task_id in table


def test_grade_reuses_caller_task_id():
    """Go 先落库再入队，taskId 由调用方给定；这里必须原样采用，不能自己换一个。"""
    store, table = fake_store()
    redis = MagicMock(lpush=AsyncMock())
    given = "0a1b2c3d4e5f60718293a4b5c6d7e8f9"
    with patch("app.api.grade.grade_store", store), patch("app.api.grade._r", redis):
        r = client.post("/api/ai/grade", json=make_body(1, taskId=given), headers=TEACHER_HEADERS)
    assert r.status_code == 200
    assert r.json()["taskId"] == given
    assert given in table
    assert json.loads(redis.lpush.call_args.args[1])["taskId"] == given


def test_grade_task_id_is_case_normalized():
    store, table = fake_store()
    redis = MagicMock(lpush=AsyncMock())
    upper = "0A1B2C3D4E5F60718293A4B5C6D7E8F9"
    with patch("app.api.grade.grade_store", store), patch("app.api.grade._r", redis):
        r = client.post("/api/ai/grade", json=make_body(1, taskId=upper), headers=TEACHER_HEADERS)
    assert r.json()["taskId"] == upper.lower()
    assert upper.lower() in table


def test_grade_rejects_task_id_with_redis_key_chars():
    """taskId 会拼进 Redis key，花括号/斜杠/空格必须挡住。"""
    store, _ = fake_store()
    redis = MagicMock(lpush=AsyncMock())
    for bad in ["{abc}", "a/b", "with space", "g" * 32, "x" * 65]:
        with patch("app.api.grade.grade_store", store), patch("app.api.grade._r", redis):
            r = client.post("/api/ai/grade", json=make_body(1, taskId=bad), headers=TEACHER_HEADERS)
        assert r.status_code == 400, bad
    redis.lpush.assert_not_called()


# ---- 任务状态查询 ----

def test_task_status_requires_user_id():
    with patch("app.api.grade.grade_store", fake_store()[0]):
        r = client.get("/api/ai/task/whatever")
    assert r.status_code == 401


def test_task_status_not_found():
    with patch("app.api.grade.grade_store", fake_store()[0]):
        r = client.get("/api/ai/task/nope", headers={"X-User-Id": "1"})
    assert r.status_code == 404
    assert "过期" in r.json()["detail"] or "不存在" in r.json()["detail"]


def test_task_status_returns_stored_task():
    store, table = fake_store()
    redis = MagicMock(lpush=AsyncMock())
    with patch("app.api.grade.grade_store", store), patch("app.api.grade._r", redis):
        r = client.post("/api/ai/grade", json=make_body(), headers=TEACHER_HEADERS)
    task_id = r.json()["taskId"]
    table[task_id].update(status="done", result={"score": 18, "maxScore": 20, "parseOk": True})

    with patch("app.api.grade.grade_store", store):
        r = client.get(f"/api/ai/task/{task_id}", headers={"X-User-Id": "1"})
    assert r.status_code == 200
    data = r.json()
    assert data["status"] == "done"
    assert data["result"]["score"] == 18
    assert data["itemCount"] == 1


def test_task_status_for_student_user_is_allowed():
    """查进度只要求登录：Go 代理会带上任意用户 id，角色不再限制。"""
    store, table = fake_store()
    with patch("app.api.grade.grade_store", store):
        r = client.get("/api/ai/task/xyz", headers={"X-User-Id": "77", "X-Role": "student"})
    assert r.status_code == 404
