"""核心生图逻辑:调用中转站的 OpenAI 兼容图片接口,保存结果到 outputs/。

支持三种请求格式(中转站对 gpt-image-2 的实现各不相同):
  - images : /v1/images/generations  标准图片接口
  - chat   : /v1/chat/completions    对话式生图(图以 markdown / base64 返回)
  - edits  : /v1/images/edits         图片编辑(需上传参考图)
"""
import asyncio
import base64
import json
import os
import re
import time
import uuid

import httpx

_RETRY_CODES = {429, 500, 502, 503, 504}
_BUSY_WORDS = ("繁忙", "busy", "try again", "rate limit", "overloaded", "稍后")


def _is_transient(resp: httpx.Response) -> bool:
    if resp.status_code in _RETRY_CODES:
        return True
    if resp.status_code == 403:
        return any(w in resp.text.lower() if w.isascii() else w in resp.text for w in _BUSY_WORDS)
    return False


async def _post_json_retry(client, endpoint, payload, headers, timeout, retries=2):
    """POST JSON,遇到瞬时错误(繁忙/限流/网关)自动重试,带退避。"""
    resp = None
    for attempt in range(retries + 1):
        resp = await client.post(endpoint, json=payload, headers=headers, timeout=timeout)
        if resp.status_code == 200 or not _is_transient(resp) or attempt == retries:
            return resp
        await asyncio.sleep(1.5 * (attempt + 1))  # 1.5s, 3s
    return resp

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
OUTPUT_DIR = os.path.join(BASE_DIR, "outputs")
os.makedirs(OUTPUT_DIR, exist_ok=True)


class GenerationError(Exception):
    """带可读信息的生图错误。"""


def _save_png(data: bytes) -> str:
    """保存 PNG 到 outputs/,返回文件名。"""
    name = f"{time.strftime('%Y%m%d-%H%M%S')}-{uuid.uuid4().hex[:8]}.png"
    with open(os.path.join(OUTPUT_DIR, name), "wb") as f:
        f.write(data)
    return name


async def _normalize_items(items: list, client: httpx.AsyncClient) -> list:
    """把接口返回的 data 项统一成 {filename, b64} —— 支持 b64_json 或 url。"""
    results = []
    for item in items:
        raw = None
        if item.get("b64_json"):
            raw = base64.b64decode(item["b64_json"])
        elif item.get("url"):
            resp = await client.get(item["url"], timeout=120)
            resp.raise_for_status()
            raw = resp.content
        if raw is None:
            continue
        filename = _save_png(raw)
        results.append({
            "filename": filename,
            "url": f"/outputs/{filename}",
            "b64_json": base64.b64encode(raw).decode(),
        })
    return results


async def generate(
    *,
    prompt: str,
    base_url: str,
    api_key: str,
    model: str,
    size: str = "1024x1024",
    quality: str | None = None,
    n: int = 1,
    timeout: int = 300,
    extra: dict | None = None,
) -> list:
    """文生图。返回 [{filename, url, b64_json}, ...]。"""
    if not base_url or not api_key:
        raise GenerationError("未配置中转站地址或密钥,请先在「设置」中填写。")

    endpoint = base_url.rstrip("/") + "/v1/images/generations"
    payload = {"model": model, "prompt": prompt, "n": n, "size": size}
    if quality:
        payload["quality"] = quality
    if extra:
        payload.update(extra)

    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    async with httpx.AsyncClient() as client:
        try:
            resp = await _post_json_retry(client, endpoint, payload, headers, timeout)
        except httpx.TimeoutException:
            raise GenerationError(f"请求超时({timeout}s),高分辨率可在设置里调大超时。")
        except httpx.RequestError as e:
            raise GenerationError(f"连接中转站失败:{e}")

        if resp.status_code != 200:
            raise GenerationError(_extract_error(resp))

        try:
            body = resp.json()
        except ValueError:
            raise GenerationError(f"中转站返回非 JSON:{resp.text[:300]}")

        items = body.get("data") or []
        if not items:
            raise GenerationError(f"中转站未返回图片数据:{str(body)[:300]}")

        results = await _normalize_items(items, client)
        if not results:
            raise GenerationError("返回的数据里没有可解析的图片(无 b64_json / url)。")
        return results


async def _fetch_image(u: str, client: httpx.AsyncClient) -> bytes | None:
    """把一个 url 或 data URI 取成图片字节。"""
    if u.startswith("data:"):
        try:
            return base64.b64decode(u.split(",", 1)[1])
        except (ValueError, IndexError):
            return None
    try:
        r = await client.get(u, timeout=120)
        r.raise_for_status()
        return r.content
    except httpx.HTTPError:
        return None


def _urls_from_content(content) -> list:
    """从 chat 返回的 content(字符串或多模态列表)里抽出图片 url / data URI。"""
    out = []
    if isinstance(content, str):
        out += re.findall(r"!\[[^\]]*\]\(([^)\s]+)\)", content)              # markdown 图片
        out += re.findall(r"(data:image/[^;]+;base64,[A-Za-z0-9+/=]+)", content)  # data URI
        out += re.findall(r"(https?://[^\s)\"']+\.(?:png|jpe?g|webp|gif))", content, re.I)  # 裸链接
    elif isinstance(content, list):
        for part in content:
            if not isinstance(part, dict):
                continue
            if part.get("type") == "image_url":
                iu = part.get("image_url")
                u = iu.get("url") if isinstance(iu, dict) else iu
                if u:
                    out.append(u)
            elif part.get("type") == "text":
                out += _urls_from_content(part.get("text", ""))
    return out


def _chat_text(body: dict) -> str:
    try:
        c = body["choices"][0]["message"].get("content")
        return c if isinstance(c, str) else str(c)
    except (KeyError, IndexError, TypeError):
        return str(body)


async def _extract_images_from_chat(body: dict, client: httpx.AsyncClient) -> list:
    urls = []
    for ch in (body.get("choices") or []):
        msg = ch.get("message") or {}
        urls += _urls_from_content(msg.get("content"))
        for im in (msg.get("images") or []):  # 部分中转站把图放在 message.images
            if isinstance(im, dict):
                iu = im.get("image_url")
                u = iu.get("url") if isinstance(iu, dict) else (iu or im.get("url"))
                if u:
                    urls.append(u)
            elif isinstance(im, str):
                urls.append(im)
    # 去重保序
    seen, ordered = set(), []
    for u in urls:
        if u not in seen:
            seen.add(u)
            ordered.append(u)
    results = []
    for u in ordered:
        raw = await _fetch_image(u, client)
        if raw:
            filename = _save_png(raw)
            results.append({
                "filename": filename,
                "url": f"/outputs/{filename}",
                "b64_json": base64.b64encode(raw).decode(),
            })
    return results


async def generate_via_chat(
    *,
    prompt: str,
    base_url: str,
    api_key: str,
    model: str,
    size: str | None = None,
    quality: str | None = None,
    n: int = 1,
    timeout: int = 300,
    extra: dict | None = None,
) -> list:
    """用 /v1/chat/completions 生图,解析返回内容里的图片。"""
    if not base_url or not api_key:
        raise GenerationError("未配置中转站地址或密钥,请先在「设置」中填写。")

    endpoint = base_url.rstrip("/") + "/v1/chat/completions"
    headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
    user_prompt = prompt
    if size and size != "auto":
        user_prompt = f"{prompt}\n\n(图片尺寸 image size: {size})"

    results = []
    async with httpx.AsyncClient() as client:
        for _ in range(max(1, n)):
            payload = {"model": model, "messages": [{"role": "user", "content": user_prompt}], "stream": False}
            if extra:
                payload.update(extra)
            try:
                resp = await _post_json_retry(client, endpoint, payload, headers, timeout)
            except httpx.TimeoutException:
                raise GenerationError(f"请求超时({timeout}s)。")
            except httpx.RequestError as e:
                raise GenerationError(f"连接中转站失败:{e}")
            if resp.status_code != 200:
                raise GenerationError(_extract_error(resp))
            try:
                body = resp.json()
            except ValueError:
                raise GenerationError(f"中转站返回非 JSON:{resp.text[:300]}")
            imgs = await _extract_images_from_chat(body, client)
            if not imgs:
                raise GenerationError("对话接口未返回图片(可能该模型/中转站不支持对话生图)。返回:" + _chat_text(body)[:200])
            results.extend(imgs)
    return results


async def edit(
    *,
    prompt: str,
    images: list[tuple[str, bytes]],
    base_url: str,
    api_key: str,
    model: str,
    size: str = "1024x1024",
    quality: str | None = None,
    n: int = 1,
    timeout: int = 300,
) -> list:
    """图生图 / 编辑。images = [(filename, bytes), ...]。"""
    if not base_url or not api_key:
        raise GenerationError("未配置中转站地址或密钥,请先在「设置」中填写。")

    endpoint = base_url.rstrip("/") + "/v1/images/edits"
    headers = {"Authorization": f"Bearer {api_key}"}

    data = {"model": model, "prompt": prompt, "n": str(n), "size": size}
    if quality:
        data["quality"] = quality
    files = [("image", (name, content, "image/png")) for name, content in images]

    async with httpx.AsyncClient() as client:
        try:
            resp = await client.post(
                endpoint, data=data, files=files, headers=headers, timeout=timeout
            )
        except httpx.TimeoutException:
            raise GenerationError(f"请求超时({timeout}s)。")
        except httpx.RequestError as e:
            raise GenerationError(f"连接中转站失败:{e}")

        if resp.status_code != 200:
            raise GenerationError(_extract_error(resp))

        body = resp.json()
        items = body.get("data") or []
        results = await _normalize_items(items, client)
        if not results:
            raise GenerationError("编辑接口未返回可解析的图片。")
        return results


_HINTS = {
    401: "密钥无效或未授权,检查 API 密钥是否正确。",
    403: "被拒绝/禁止。常见原因:① 中转站上游繁忙或限流(稍后重试);② 该尺寸不被支持(先用 1024x1024 试);③ 密钥无此模型权限;④ 触发风控。",
    404: "接口不存在。可能该中转站不支持此请求格式,换「对话生图(chat)」或确认 base_url 是否正确(不要带 /v1)。",
    429: "触发限流(请求太频繁/超额)。把并发数调到 1、稍后再试,或检查额度。",
    400: "请求参数有误。最可能是尺寸不被支持(很多 gpt-image 只支持 1024x1024 / 1536x1024 / 1024x1536)。",
    500: "中转站上游内部错误,通常稍后重试可恢复。",
    502: "网关错误,中转站到上游的连接出问题,稍后重试。",
    503: "服务不可用,上游繁忙,稍后重试。",
    504: "网关超时,上游响应太慢,稍后重试或调大超时。",
}


async def reverse_prompt(
    *,
    image: bytes,
    base_url: str,
    api_key: str,
    model: str,
    timeout: int = 120,
    instruction: str | None = None,
) -> str:
    """参考图反推:把图片喂给视觉模型,返回可直接拿去生图的提示词。"""
    if not base_url or not api_key:
        raise GenerationError("未配置中转站地址或密钥,请先在「设置」中填写。")

    endpoint = base_url.rstrip("/") + "/v1/chat/completions"
    headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
    data_uri = "data:image/png;base64," + base64.b64encode(image).decode()
    instr = instruction or (
        "请仔细观察这张图片,用一段详细、可直接用于 AI 生图的提示词来描述它,"
        "涵盖主体、风格、构图、配色、光影、材质、氛围等。只输出提示词本身,不要解释、不要前后缀。"
    )
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": [
            {"type": "text", "text": instr},
            {"type": "image_url", "image_url": {"url": data_uri}},
        ]}],
        "stream": False,
    }
    async with httpx.AsyncClient() as client:
        try:
            resp = await _post_json_retry(client, endpoint, payload, headers, timeout)
        except httpx.TimeoutException:
            raise GenerationError(f"请求超时({timeout}s)。")
        except httpx.RequestError as e:
            raise GenerationError(f"连接中转站失败:{e}")
        if resp.status_code != 200:
            raise GenerationError(_extract_error(resp))
        try:
            text = _chat_text(resp.json()).strip()
        except ValueError:
            raise GenerationError(f"中转站返回非 JSON:{resp.text[:300]}")
        if not text:
            raise GenerationError("反推失败:模型没有返回文本(该模型可能不支持看图)。")
        return text


def _extract_error(resp: httpx.Response) -> str:
    """从错误响应里提取尽量详细可读的信息:状态码 + 原始 message + 字段 + request-id + 解决提示。"""
    code = resp.status_code
    rid = resp.headers.get("x-request-id") or resp.headers.get("cf-ray") or ""
    detail = ""
    try:
        body = resp.json()
        err = body.get("error", body) if isinstance(body, dict) else body
        if isinstance(err, dict):
            parts = []
            for k in ("message", "type", "code", "param"):
                if err.get(k):
                    parts.append(f"{k}={err[k]}")
            detail = " | ".join(parts) if parts else json.dumps(err, ensure_ascii=False)[:400]
        else:
            detail = str(err)[:400]
    except ValueError:
        detail = (resp.text[:400] or "(空响应体)")

    msg = f"[HTTP {code}] {detail}"
    if rid:
        msg += f"\nrequest-id: {rid}"
    if code in _HINTS:
        msg += f"\n💡 {_HINTS[code]}"
    return msg
