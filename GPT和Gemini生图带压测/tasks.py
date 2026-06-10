"""异步任务队列:批量生图任务排队、顺序执行、可查询进度。

任务完成后写入历史(store.add_history)。仅文生图走队列;
图生图(带文件上传)由 app 直接同步调用。
"""
import asyncio
import time
import uuid
from collections import OrderedDict

import config
import generator
import store

# 任务状态: queued -> running -> done | error
_tasks: "OrderedDict[str, dict]" = OrderedDict()
_queue: asyncio.Queue | None = None
_worker_started = False
_running_workers = 0  # 当前实际运行的 worker 数(并发数)
MAX_KEEP = 100  # 内存中最多保留的任务记录数


def running_workers() -> int:
    return _running_workers


def _now() -> int:
    return int(time.time())


def enqueue(*, prompt, size, quality, n, model=None, request_format="images") -> dict:
    """加入一个文生图任务,返回任务记录。"""
    task = {
        "id": uuid.uuid4().hex[:12],
        "mode": "gen",
        "status": "queued",
        "prompt": prompt,
        "size": size,
        "quality": quality,
        "n": n,
        "model": model,
        "request_format": request_format,
        "created_at": _now(),
        "started_at": None,
        "finished_at": None,
        "images": [],
        "error": None,
    }
    _tasks[task["id"]] = task
    # 超出上限时清理最旧的已结束任务
    while len(_tasks) > MAX_KEEP:
        for tid, t in list(_tasks.items()):
            if t["status"] in ("done", "error"):
                _tasks.pop(tid, None)
                break
        else:
            break
    assert _queue is not None, "队列未启动"
    _queue.put_nowait(task["id"])
    return task


def get(task_id: str) -> dict | None:
    return _tasks.get(task_id)


def list_tasks(limit: int = 50) -> list:
    items = list(_tasks.values())[-limit:]
    return list(reversed(items))


async def _run_one(task_id: str):
    task = _tasks.get(task_id)
    if not task:
        return
    task["status"] = "running"
    task["started_at"] = _now()
    cfg = config.load()
    fmt = task.get("request_format") or cfg.get("request_format", "images")
    try:
        common = dict(
            prompt=task["prompt"],
            base_url=cfg["base_url"],
            api_key=cfg["api_key"],
            model=task["model"] or cfg["model"],
            size=task["size"] or cfg["default_size"],
            quality=task["quality"] or cfg["default_quality"],
            n=task["n"],
            timeout=int(cfg.get("timeout", 300)),
        )
        if fmt == "chat":
            results = await generator.generate_via_chat(**common)
        else:
            results = await generator.generate(**common)
        task["images"] = [{"filename": r["filename"], "url": r["url"]} for r in results]
        task["status"] = "done"
        store.add_history(
            mode="gen", prompt=task["prompt"], model=task["model"] or cfg["model"],
            size=task["size"], quality=task["quality"], n=task["n"],
            files=[r["filename"] for r in results],
        )
    except generator.GenerationError as e:
        task["status"] = "error"
        task["error"] = str(e)
    except Exception as e:  # 兜底,避免 worker 因单个任务崩溃
        task["status"] = "error"
        task["error"] = f"内部错误:{e}"
    finally:
        task["finished_at"] = _now()


async def _worker(worker_id: int):
    while True:
        task_id = await _queue.get()
        try:
            await _run_one(task_id)
        finally:
            _queue.task_done()


def start():
    """在 FastAPI startup 时调用,初始化队列并按并发数启动多个 worker。"""
    global _queue, _worker_started, _running_workers
    if _worker_started:
        return
    _queue = asyncio.Queue()
    concurrency = max(1, min(int(config.load().get("concurrency", 3)), 16))
    for i in range(concurrency):
        asyncio.create_task(_worker(i))
    _running_workers = concurrency
    _worker_started = True
