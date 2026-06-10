"""生图压测:按自定义并发向中转站发起 N 次真实生图请求,统计成功率/延迟/吞吐。

注意:压测会真实调用中转站、产生计费/消耗额度。为避免占满磁盘,压测只测量
不保存图片。结果保存在内存,前端轮询 /api/stress/status。

并发无硬上限:压测跑在**独立线程 + Proactor(IOCP)事件循环**里,绕开主服务
select 循环的 512 套接字限制,且与网页服务互不干扰(主服务照常用 select)。
"""
import asyncio
import sys
import threading
import time

import httpx

import config
import generator

_run: dict | None = None        # 当前压测状态(跨线程共享)
_loop = None                    # 压测线程的事件循环(供主线程取消任务用)
_thread = None                  # 压测线程
_lock = threading.Lock()        # 保护 _run 里计数/集合的读写


def state() -> dict | None:
    return _run


async def _one(client, endpoint, payload, headers, timeout, fmt):
    """发一次请求,返回 (ok, latency_ms, status_code, error)。压测不重试,要看真实结果。"""
    start = time.monotonic()
    try:
        resp = await client.post(endpoint, json=payload, headers=headers, timeout=timeout)
        lat = (time.monotonic() - start) * 1000
        if resp.status_code != 200:
            line = generator._extract_error(resp).splitlines()[0][:160]
            return False, lat, resp.status_code, line
        ok = True
        try:
            body = resp.json()
            if fmt != "chat":
                ok = bool(body.get("data"))
        except ValueError:
            ok = fmt == "chat"
        return ok, lat, 200, (None if ok else "200 但无图片数据")
    except httpx.TimeoutException:
        return False, (time.monotonic() - start) * 1000, 0, "超时"
    except httpx.RequestError as e:
        return False, (time.monotonic() - start) * 1000, 0, f"连接失败:{e}"


async def _run_all(endpoint, payload, headers, timeout, total, concurrency, fmt):
    """工作池:只开 concurrency 个 worker,各自循环领取任务,真正限制同时在飞数。"""
    limits = httpx.Limits(max_connections=concurrency, max_keepalive_connections=concurrency)
    dispatched = 0  # 同一事件循环线程内,「判断+自增」之间无 await,天然原子

    async with httpx.AsyncClient(limits=limits) as client:
        async def worker():
            nonlocal dispatched
            while not _run["cancel"]:
                if dispatched >= total:
                    return
                dispatched += 1
                ok, lat, status, err = await _one(client, endpoint, payload, headers, timeout, fmt)
                with _lock:
                    _run["done"] += 1
                    _run["latencies"].append(lat)
                    if ok:
                        _run["ok"] += 1
                    else:
                        _run["fail"] += 1
                        label = f"[{status}] {err}" if status else str(err)
                        _run["errors"][label] = _run["errors"].get(label, 0) + 1

        tasks = [asyncio.create_task(worker()) for _ in range(concurrency)]
        _run["_tasks"] = tasks
        await asyncio.gather(*tasks, return_exceptions=True)


def start(*, prompt, total, concurrency, model, size, quality, fmt):
    """启动一轮压测:在独立线程里用 Proactor 循环跑,绕过主服务 select 的 512 限制。"""
    global _run, _loop, _thread
    cfg = config.load()
    base, key = cfg.get("base_url"), cfg.get("api_key")
    if not base or not key:
        _run = {"status": "error", "error": "未配置中转站地址或密钥", "done": 0, "total": total}
        return

    endpoint = base.rstrip("/") + ("/v1/chat/completions" if fmt == "chat" else "/v1/images/generations")
    headers = {"Authorization": f"Bearer {key}", "Content-Type": "application/json"}
    if fmt == "chat":
        payload = {"model": model, "messages": [{"role": "user", "content": prompt}], "stream": False}
    else:
        payload = {"model": model, "prompt": prompt, "n": 1, "size": size}
        if quality:
            payload["quality"] = quality
    timeout = int(cfg.get("timeout", 300))
    concurrency = min(concurrency, total)  # 并发不必超过总数

    _run = {
        "status": "running", "total": total, "concurrency": concurrency,
        "model": model, "size": size, "fmt": fmt,
        "done": 0, "ok": 0, "fail": 0, "latencies": [], "errors": {},
        "start_mono": time.monotonic(), "started_at": int(time.time()),
        "cancel": False, "_tasks": [],
    }

    def thread_main():
        global _loop
        loop = asyncio.ProactorEventLoop() if sys.platform == "win32" else asyncio.new_event_loop()
        _loop = loop
        asyncio.set_event_loop(loop)
        try:
            loop.run_until_complete(_run_all(endpoint, payload, headers, timeout, total, concurrency, fmt))
        except Exception as e:  # 兜底,别让线程静默死掉
            _run["status"] = "error"
            _run["error"] = f"压测线程异常:{e}"
        finally:
            try:
                loop.close()
            except Exception:
                pass
            _loop = None
            if _run.get("status") == "running":
                _run["status"] = "cancelled" if _run.get("cancel") else "done"
            _run["elapsed"] = time.monotonic() - _run["start_mono"]
            _run.pop("_tasks", None)

    _thread = threading.Thread(target=thread_main, daemon=True)
    _thread.start()


def cancel():
    """停止:置取消标志。worker 跑完当前这次请求后就退出,连接干净关闭。

    不用跨线程 task.cancel():在 Proactor 循环上强行取消在飞的 httpx 请求会让
    连接池清理卡死。靠标志位让 worker 自然收尾,最多多等一次请求的时间。
    """
    if _run and _run.get("status") == "running":
        _run["cancel"] = True


def _pct(sorted_lats, p):
    if not sorted_lats:
        return 0
    idx = min(len(sorted_lats) - 1, max(0, int(p / 100 * len(sorted_lats))))
    return sorted_lats[idx]


def stats() -> dict:
    """计算当前压测的统计数据(供 /api/stress/status 返回)。对任何 _run 形态都安全。"""
    if not _run:
        return {"status": "idle"}
    # 压测线程在另一个线程写 _run,这里加锁快照,避免「list/dict changed during iteration」
    with _lock:
        lats = sorted(_run.get("latencies", []))
        errors = dict(_run.get("errors", {}))
        done = _run.get("done", 0)
        ok = _run.get("ok", 0)
        fail = _run.get("fail", 0)
    start_mono = _run.get("start_mono")
    elapsed = _run.get("elapsed") or ((time.monotonic() - start_mono) if start_mono else 0)
    return {
        "status": _run.get("status", "idle"),
        "total": _run.get("total", 0),
        "concurrency": _run.get("concurrency", 0),
        "model": _run.get("model", ""), "size": _run.get("size", ""), "fmt": _run.get("fmt", ""),
        "done": done, "ok": ok, "fail": fail,
        "elapsed": round(elapsed, 2),
        "rps": round(done / elapsed, 2) if elapsed > 0 else 0,
        "lat_min": round(min(lats), 0) if lats else 0,
        "lat_max": round(max(lats), 0) if lats else 0,
        "lat_avg": round(sum(lats) / len(lats), 0) if lats else 0,
        "lat_p50": round(_pct(lats, 50), 0),
        "lat_p95": round(_pct(lats, 95), 0),
        "errors": errors,
        "error": _run.get("error"),
    }
