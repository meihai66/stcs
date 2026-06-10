"""配置读写:支持多套中转站「配置(profile)」,可保存多个、随时切换。

config.json 结构:
{
  "profiles": [
    {"name":"中转站A", "base_url":"...", "api_key":"...", "model":"gpt-image-2",
     "reverse_model":"gpt-4o", "request_format":"images"},
    ...
  ],
  "active_profile": "中转站A",
  "default_size":"1024x1024", "default_quality":"high",
  "timeout":300, "concurrency":3, "server_api_key":""
}

load() 仍返回「扁平」配置(全局项 + 当前激活 profile 的中转站字段),
所以 generator / tasks / stress 等无需改动。
"""
import json
import os
import threading

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
# 可用环境变量自定义配置文件位置(测试 / 多实例用),默认项目根目录 config.json
CONFIG_PATH = os.environ.get("GPTIMG_CONFIG_PATH") or os.path.join(BASE_DIR, "config.json")

# 每套中转站独有的字段
PROFILE_FIELDS = ["base_url", "api_key", "model", "reverse_model", "request_format"]
PROFILE_DEFAULTS = {
    "base_url": "", "api_key": "", "model": "gpt-image-2",
    "reverse_model": "gpt-4o", "request_format": "images",
}
# 全局共享字段(不随 profile 切换)
GLOBAL_DEFAULTS = {
    "default_size": "1024x1024", "default_quality": "high",
    "timeout": 300, "concurrency": 3, "server_api_key": "",
}

_lock = threading.Lock()
_cache = None


def _read_raw() -> dict:
    """读取 config.json 并做结构迁移(旧的扁平格式 → profiles)。"""
    raw = {}
    if os.path.exists(CONFIG_PATH):
        try:
            with open(CONFIG_PATH, "r", encoding="utf-8") as f:
                raw = json.load(f)
        except (json.JSONDecodeError, OSError):
            raw = {}
    if not isinstance(raw, dict):
        raw = {}
    # 迁移:旧版是扁平的 base_url/api_key/...,包成一个「默认」profile
    if not isinstance(raw.get("profiles"), list):
        prof = {f: raw.get(f, PROFILE_DEFAULTS[f]) for f in PROFILE_FIELDS}
        prof["name"] = raw.get("active_profile") or "默认"
        has_data = bool(prof["base_url"] or prof["api_key"])
        raw["profiles"] = [prof] if has_data else []
        raw["active_profile"] = prof["name"] if has_data else ""
    for k, v in GLOBAL_DEFAULTS.items():
        raw.setdefault(k, v)
    if not raw.get("active_profile") and raw["profiles"]:
        raw["active_profile"] = raw["profiles"][0].get("name", "")
    return raw


def _active(raw: dict) -> dict:
    name = raw.get("active_profile")
    for p in raw["profiles"]:
        if p.get("name") == name:
            return p
    return raw["profiles"][0] if raw["profiles"] else dict(PROFILE_DEFAULTS, name="")


def _write_raw(raw: dict):
    with open(CONFIG_PATH, "w", encoding="utf-8") as f:
        json.dump(raw, f, ensure_ascii=False, indent=2)


def load() -> dict:
    """返回扁平配置(全局项 + 当前 profile 的中转站字段),环境变量优先。"""
    global _cache
    with _lock:
        raw = _read_raw()
        active = _active(raw)
        cfg = {k: raw.get(k, v) for k, v in GLOBAL_DEFAULTS.items()}
        for f in PROFILE_FIELDS:
            cfg[f] = active.get(f, PROFILE_DEFAULTS[f])
        cfg["active_profile"] = raw.get("active_profile", "")
        # 环境变量覆盖(测试/部署用)
        env_map = {
            "GPTIMG_BASE_URL": "base_url", "GPTIMG_API_KEY": "api_key",
            "GPTIMG_MODEL": "model", "GPTIMG_SERVER_API_KEY": "server_api_key",
        }
        for env_key, cfg_key in env_map.items():
            val = os.environ.get(env_key)
            if val:
                cfg[cfg_key] = val
        _cache = cfg
        return dict(cfg)


def save(updates: dict) -> dict:
    """保存全局项;中转站字段写入当前激活的 profile(无则建「默认」)。"""
    with _lock:
        raw = _read_raw()
        for k in GLOBAL_DEFAULTS:
            if k in updates and updates[k] is not None:
                raw[k] = updates[k]
        relay = {f: updates[f] for f in PROFILE_FIELDS
                 if f in updates and updates[f] is not None}
        if relay:
            if not raw["profiles"]:
                raw["profiles"] = [dict(PROFILE_DEFAULTS, name="默认")]
                raw["active_profile"] = "默认"
            _active(raw).update(relay)
        _write_raw(raw)
    return load()


# ----------------------------- 多 profile 管理 -----------------------------

def list_profiles() -> dict:
    with _lock:
        raw = _read_raw()
        return {"profiles": [dict(p) for p in raw["profiles"]],
                "active": raw.get("active_profile", "")}


def save_profile(profile: dict) -> dict:
    """新增或更新一套中转站配置(按 name)。api_key 为空或脱敏占位符时不覆盖原值。"""
    name = (profile.get("name") or "").strip()
    if not name:
        raise ValueError("配置名不能为空")
    with _lock:
        raw = _read_raw()
        prof = next((p for p in raw["profiles"] if p.get("name") == name), None)
        if prof is None:
            prof = dict(PROFILE_DEFAULTS, name=name)
            raw["profiles"].append(prof)
        for f in PROFILE_FIELDS:
            if f not in profile or profile[f] is None:
                continue
            if f == "api_key" and (not profile[f] or str(profile[f]).startswith("****")):
                continue  # 不用脱敏占位符覆盖真实密钥
            prof[f] = profile[f]
        if not raw.get("active_profile"):
            raw["active_profile"] = name
        _write_raw(raw)
    return list_profiles()


def delete_profile(name: str) -> dict:
    with _lock:
        raw = _read_raw()
        raw["profiles"] = [p for p in raw["profiles"] if p.get("name") != name]
        if raw.get("active_profile") == name:
            raw["active_profile"] = raw["profiles"][0]["name"] if raw["profiles"] else ""
        _write_raw(raw)
    return list_profiles()


def set_active(name: str) -> dict:
    with _lock:
        raw = _read_raw()
        if any(p.get("name") == name for p in raw["profiles"]):
            raw["active_profile"] = name
            _write_raw(raw)
    return load()


def get() -> dict:
    """返回缓存配置;首次调用会加载。"""
    return load() if _cache is None else dict(_cache)
