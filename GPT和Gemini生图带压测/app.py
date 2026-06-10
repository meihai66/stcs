"""本地生图服务:Web UI + OpenAI 兼容 API。

启动:  python -m uvicorn app:app --host 127.0.0.1 --port 5311
        或   python app.py
"""
import asyncio
import os
import sys
import time

import httpx
from fastapi import FastAPI, Form, Header, HTTPException, Request, UploadFile
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

import config
import generator
import store
import stress
import tasks

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
STATIC_DIR = os.path.join(BASE_DIR, "static")
OUTPUT_DIR = generator.OUTPUT_DIR

app = FastAPI(title="本地 GPT 生图", version="2.0.0")


@app.on_event("startup")
async def _startup():
    tasks.start()


# ----------------------------- Web UI 接口 -----------------------------

@app.get("/")
async def index():
    return FileResponse(
        os.path.join(STATIC_DIR, "index.html"),
        headers={"Cache-Control": "no-cache, no-store, must-revalidate"},
    )


@app.get("/api/config")
async def get_config():
    """返回配置,密钥脱敏(只回显是否已设置 + 末 4 位)。"""
    cfg = config.load()
    masked = dict(cfg)
    key = cfg.get("api_key") or ""
    masked["api_key"] = (f"****{key[-4:]}" if len(key) >= 4 else "") if key else ""
    masked["has_api_key"] = bool(key)
    skey = cfg.get("server_api_key") or ""
    masked["server_api_key"] = "****" if skey else ""
    masked["has_server_api_key"] = bool(skey)
    masked["running_workers"] = tasks.running_workers()
    masked["active_profile"] = cfg.get("active_profile", "")
    return masked


def _mask_key(k: str) -> str:
    return (f"****{k[-4:]}" if len(k) >= 4 else "****") if k else ""


@app.get("/api/profiles")
async def api_list_profiles():
    """列出所有中转站配置(密钥脱敏)+ 当前激活的。"""
    data = config.list_profiles()
    out = []
    for p in data["profiles"]:
        out.append({
            "name": p.get("name", ""),
            "base_url": p.get("base_url", ""),
            "api_key": _mask_key(p.get("api_key", "")),
            "has_api_key": bool(p.get("api_key")),
            "model": p.get("model", ""),
            "reverse_model": p.get("reverse_model", ""),
            "request_format": p.get("request_format", "images"),
        })
    return {"profiles": out, "active": data["active"]}


class ProfileBody(BaseModel):
    name: str
    base_url: str | None = None
    api_key: str | None = None
    model: str | None = None
    reverse_model: str | None = None
    request_format: str | None = None


@app.post("/api/profiles")
async def api_save_profile(body: ProfileBody):
    try:
        config.save_profile(body.model_dump())
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    return {"ok": True}


@app.post("/api/profiles/activate")
async def api_activate_profile(payload: dict):
    name = (payload.get("name") or "").strip()
    cfg = config.set_active(name)
    return {"ok": True, "active": cfg.get("active_profile"), "model": cfg.get("model")}


@app.delete("/api/profiles/{name}")
async def api_delete_profile(name: str):
    config.delete_profile(name)
    return {"ok": True}


@app.post("/api/config")
async def set_config(payload: dict):
    """保存设置。空字符串的 api_key 表示「不修改」,避免脱敏回显覆盖真实值。"""
    updates = {}
    for key in ("base_url", "model", "reverse_model", "request_format",
                "default_size", "default_quality", "timeout", "concurrency"):
        if key in payload:
            updates[key] = payload[key]
    # 密钥:仅当传入非空且不是脱敏占位符时才更新
    for key in ("api_key", "server_api_key"):
        val = payload.get(key)
        if val and not val.startswith("****"):
            updates[key] = val
    cfg = config.save(updates)
    return {"ok": True, "model": cfg.get("model")}


class GenerateBody(BaseModel):
    prompt: str | None = None          # 兼容单条;前端现在传 prompts
    prompts: list[str] | None = None   # 多条提示词,每条一个独立任务(并行执行)
    size: str | None = None
    quality: str | None = None
    n: int = 1
    repeat: int = 1        # 每条提示词重复排几个任务
    request_format: str | None = None   # images | chat
    model: str | None = None


@app.post("/api/generate")
async def api_generate(body: GenerateBody):
    """多条提示词,每条一个独立任务并行执行。前端轮询 /api/tasks 看进度。"""
    cfg = config.load()
    # 收集提示词:优先 prompts 列表,否则把单条 prompt 按换行拆分
    raw = body.prompts if body.prompts is not None else [(body.prompt or "")]
    prompts = [p.strip() for p in raw if p and p.strip()]
    if not prompts:
        raise HTTPException(status_code=400, detail="请至少输入一条提示词")
    if len(prompts) > 50:
        raise HTTPException(status_code=400, detail="一次最多 50 条提示词")
    repeat = max(1, min(body.repeat, 20))
    fmt = body.request_format or cfg.get("request_format", "images")

    created = []
    for prompt in prompts:
        for _ in range(repeat):
            task = tasks.enqueue(
                prompt=prompt,
                size=body.size or cfg["default_size"],
                quality=body.quality or cfg["default_quality"],
                n=max(1, min(body.n, 4)),
                model=body.model or cfg["model"],
                request_format=fmt,
            )
            created.append({"id": task["id"], "status": task["status"]})
    return {"tasks": created}


@app.get("/api/tasks")
async def api_tasks():
    return {"tasks": tasks.list_tasks()}


@app.get("/api/tasks/{task_id}")
async def api_task(task_id: str):
    t = tasks.get(task_id)
    if not t:
        raise HTTPException(status_code=404, detail="任务不存在")
    return t


def _safe_output_path(filename: str) -> str | None:
    """把 outputs/ 内的文件名解析成安全的绝对路径,防目录穿越。"""
    name = os.path.basename(filename or "")
    if not name:
        return None
    path = os.path.join(OUTPUT_DIR, name)
    if os.path.abspath(path).startswith(os.path.abspath(OUTPUT_DIR)) and os.path.exists(path):
        return path
    return None


@app.post("/api/edit")
async def api_edit(
    prompt: str = Form(...),
    size: str = Form(None),
    quality: str = Form(None),
    n: int = Form(1),
    model: str = Form(None),
    source: str = Form(None),   # 逗号分隔的 outputs/ 文件名,用于「在此结果上继续创作」
    images: list[UploadFile] = None,
):
    cfg = config.load()
    files = []
    for up in images or []:
        files.append((up.filename or "image.png", await up.read()))
    # 追加来自 outputs/ 的已有图片(迭代/多轮创作)
    for fn in (source or "").split(","):
        fn = fn.strip()
        if not fn:
            continue
        path = _safe_output_path(fn)
        if path:
            with open(path, "rb") as f:
                files.append((os.path.basename(path), f.read()))
    if not files:
        raise HTTPException(status_code=400, detail="请上传至少一张图片(或选择一张已有结果继续)。")
    use_model = model or cfg["model"]
    try:
        results = await generator.edit(
            prompt=prompt,
            images=files,
            base_url=cfg["base_url"],
            api_key=cfg["api_key"],
            model=use_model,
            size=size or cfg["default_size"],
            quality=quality or cfg["default_quality"],
            n=max(1, min(n, 4)),
            timeout=int(cfg.get("timeout", 300)),
        )
    except generator.GenerationError as e:
        raise HTTPException(status_code=400, detail=str(e))
    store.add_history(
        mode="edit", prompt=prompt, model=use_model,
        size=size or cfg["default_size"], quality=quality or cfg["default_quality"],
        n=len(results), files=[r["filename"] for r in results],
    )
    return {"images": [{"filename": r["filename"], "url": r["url"]} for r in results]}


# ----------------------------- 历史 / 收藏 -----------------------------

@app.get("/api/history")
async def api_history(limit: int = 100):
    return {"history": store.list_history(limit)}


@app.delete("/api/history/{item_id}")
async def api_delete_history(item_id: int, with_files: bool = False):
    files = store.delete_history(item_id)
    if with_files:
        for f in files:
            try:
                os.remove(os.path.join(OUTPUT_DIR, f))
            except OSError:
                pass
    return {"ok": True}


class FavoriteBody(BaseModel):
    prompt: str
    name: str = ""


@app.get("/api/favorites")
async def api_favorites():
    return {"favorites": store.list_favorites()}


@app.post("/api/favorites")
async def api_add_favorite(body: FavoriteBody):
    if not body.prompt.strip():
        raise HTTPException(status_code=400, detail="提示词为空")
    fid = store.add_favorite(body.prompt.strip(), body.name.strip())
    return {"ok": True, "id": fid}


@app.delete("/api/favorites/{fav_id}")
async def api_delete_favorite(fav_id: int):
    store.delete_favorite(fav_id)
    return {"ok": True}


# ----------------------------- 参考图反推 -----------------------------

@app.post("/api/reverse-prompt")
async def api_reverse_prompt(image: UploadFile, model: str = Form(None)):
    cfg = config.load()
    raw = await image.read()
    if not raw:
        raise HTTPException(status_code=400, detail="未收到图片")
    try:
        text = await generator.reverse_prompt(
            image=raw,
            base_url=cfg["base_url"],
            api_key=cfg["api_key"],
            model=model or cfg.get("reverse_model") or "gpt-4o",
            timeout=int(cfg.get("timeout", 300)),
        )
    except generator.GenerationError as e:
        raise HTTPException(status_code=400, detail=str(e))
    return {"prompt": text}


# ----------------------------- 提示词市场 -----------------------------

_PROMPT_SOURCES = {
    "banana": "https://cdn.jsdelivr.net/gh/glidea/banana-prompt-quicker@main/prompts.json",
}
_market_cache: dict = {}  # url -> (ts, data)


@app.get("/api/prompt-market")
async def api_prompt_market(source: str = "banana"):
    """拉取并缓存社区精选提示词(默认 banana-prompt-quicker)。"""
    url = _PROMPT_SOURCES.get(source)
    if not url:
        raise HTTPException(status_code=400, detail="未知来源")
    cached = _market_cache.get(url)
    if cached and (time.time() - cached[0] < 3600):  # 缓存 1 小时
        return {"source": source, "prompts": cached[1], "cached": True}
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(url, timeout=30)
            resp.raise_for_status()
            data = resp.json()
    except (httpx.HTTPError, ValueError) as e:
        if cached:
            return {"source": source, "prompts": cached[1], "cached": True, "stale": True}
        raise HTTPException(status_code=502, detail=f"拉取提示词市场失败:{e}")
    # 精简字段
    out = []
    for x in data:
        if not isinstance(x, dict) or not x.get("prompt"):
            continue
        out.append({
            "title": x.get("title", ""),
            "prompt": x.get("prompt", ""),
            "preview": x.get("preview", ""),
            "author": x.get("author", ""),
            "mode": x.get("mode", "generate"),
            "category": x.get("category", ""),
            "reference_image_urls": x.get("reference_image_urls", []),
        })
    _market_cache[url] = (time.time(), out)
    return {"source": source, "prompts": out, "cached": False}


# ----------------------------- 生图压测 -----------------------------

class StressBody(BaseModel):
    prompt: str = "a cute cat"
    total: int = 20
    concurrency: int = 5
    model: str | None = None
    size: str = "1024x1024"
    quality: str | None = None
    request_format: str | None = None


# 压测跑在独立线程的 Proactor 循环里,绕过主服务 select 的 512 限制,所以并发无硬上限。
# 仅保留一个极高的兜底值,防止误填天文数字耗尽内存/端口(实际远到不了)。
STRESS_MAX_CONCURRENCY = 20000


@app.post("/api/stress/start")
async def api_stress_start(body: StressBody):
    cur = stress.state()
    if cur and cur.get("status") == "running":
        raise HTTPException(status_code=409, detail="已有压测在运行,请先停止或等待完成。")
    cfg = config.load()
    total = max(1, body.total)
    requested = max(1, body.concurrency)
    concurrency = min(requested, STRESS_MAX_CONCURRENCY)
    capped = concurrency < requested
    stress.start(
        prompt=body.prompt or "a cute cat",
        total=total,
        concurrency=concurrency,
        model=body.model or cfg["model"],
        size=body.size or "1024x1024",
        quality=body.quality or cfg.get("default_quality"),
        fmt=body.request_format or cfg.get("request_format", "images"),
    )
    return {"ok": True, "total": total, "concurrency": concurrency,
            "requested": requested, "capped": capped, "cap": STRESS_MAX_CONCURRENCY}


@app.get("/api/stress/status")
async def api_stress_status():
    return stress.stats()


@app.post("/api/stress/stop")
async def api_stress_stop():
    stress.cancel()
    return {"ok": True}


@app.get("/api/gallery")
async def gallery():
    """返回 outputs/ 里最新的图片(倒序)。"""
    files = [f for f in os.listdir(OUTPUT_DIR) if f.lower().endswith(".png")]
    files.sort(reverse=True)
    return {"images": [{"filename": f, "url": f"/outputs/{f}"} for f in files[:60]]}


# ----------------------- OpenAI 兼容 API (对外) -----------------------

def _check_server_key(auth: str | None):
    cfg = config.load()
    required = cfg.get("server_api_key")
    if not required:
        return  # 未设置则不校验
    token = (auth or "").replace("Bearer ", "").strip()
    if token != required:
        raise HTTPException(status_code=401, detail="无效的 API Key")


@app.get("/v1/models")
async def list_models(authorization: str = Header(None)):
    _check_server_key(authorization)
    cfg = config.load()
    return {
        "object": "list",
        "data": [{"id": cfg.get("model", "gpt-image-2"), "object": "model", "owned_by": "local"}],
    }


@app.post("/v1/images/generations")
async def openai_generations(req: Request, authorization: str = Header(None)):
    _check_server_key(authorization)
    payload = await req.json()
    cfg = config.load()
    prompt = payload.get("prompt")
    if not prompt:
        raise HTTPException(status_code=400, detail="缺少 prompt")
    response_format = payload.get("response_format", "b64_json")
    try:
        results = await generator.generate(
            prompt=prompt,
            base_url=cfg["base_url"],
            api_key=cfg["api_key"],
            model=payload.get("model") or cfg["model"],
            size=payload.get("size") or cfg["default_size"],
            quality=payload.get("quality") or cfg["default_quality"],
            n=int(payload.get("n", 1)),
            timeout=int(cfg.get("timeout", 300)),
        )
    except generator.GenerationError as e:
        raise HTTPException(status_code=400, detail=str(e))

    store.add_history(
        mode="api", prompt=prompt, model=payload.get("model") or cfg["model"],
        size=payload.get("size") or cfg["default_size"],
        quality=payload.get("quality") or cfg["default_quality"],
        n=len(results), files=[r["filename"] for r in results],
    )
    if response_format == "url":
        data = [{"url": r["url"]} for r in results]
    else:
        data = [{"b64_json": r["b64_json"]} for r in results]
    return {"created": int(time.time()), "data": data}


# ----------------------------- 静态资源 -----------------------------

app.mount("/outputs", StaticFiles(directory=OUTPUT_DIR), name="outputs")
app.mount("/static", StaticFiles(directory=STATIC_DIR), name="static")


@app.exception_handler(HTTPException)
async def http_exc_handler(request: Request, exc: HTTPException):
    return JSONResponse(status_code=exc.status_code, content={"error": {"message": exc.detail}})


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="127.0.0.1", port=5311)
