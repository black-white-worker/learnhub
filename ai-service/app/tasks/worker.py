"""AI Worker 独立入口：只跑批改消费者，不跑 HTTP 服务。

docker-compose 里用同镜像不同 command 起第二个容器：
  api:    uvicorn app.main:app --port 8000
  worker: python -m app.tasks.worker

这样批改任务不会阻塞 HTTP 事件循环。
"""
import asyncio
from app.tasks.grade_worker import consume_loop


async def main():
    print("[ai-worker] 启动批改消费者进程...")
    await consume_loop()


if __name__ == "__main__":
    asyncio.run(main())
