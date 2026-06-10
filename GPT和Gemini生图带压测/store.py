"""SQLite 持久化:生图历史 + 提示词收藏。"""
import json
import os
import sqlite3
import threading
import time

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DB_PATH = os.path.join(BASE_DIR, "data.db")

_lock = threading.Lock()
_conn = None


def _db() -> sqlite3.Connection:
    global _conn
    if _conn is None:
        _conn = sqlite3.connect(DB_PATH, check_same_thread=False)
        _conn.row_factory = sqlite3.Row
        _conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS history (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                created_at INTEGER NOT NULL,
                mode TEXT NOT NULL,
                prompt TEXT NOT NULL,
                model TEXT,
                size TEXT,
                quality TEXT,
                n INTEGER,
                files TEXT          -- JSON: ["20240101-...png", ...]
            );
            CREATE TABLE IF NOT EXISTS favorites (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                created_at INTEGER NOT NULL,
                name TEXT,
                prompt TEXT NOT NULL
            );
            """
        )
        _conn.commit()
    return _conn


# ----------------------------- 历史 -----------------------------

def add_history(*, mode, prompt, model, size, quality, n, files) -> int:
    with _lock:
        cur = _db().execute(
            "INSERT INTO history (created_at, mode, prompt, model, size, quality, n, files)"
            " VALUES (?,?,?,?,?,?,?,?)",
            (int(time.time()), mode, prompt, model, size, quality, n, json.dumps(files)),
        )
        _db().commit()
        return cur.lastrowid


def list_history(limit: int = 100) -> list:
    with _lock:
        rows = _db().execute(
            "SELECT * FROM history ORDER BY id DESC LIMIT ?", (limit,)
        ).fetchall()
    out = []
    for r in rows:
        files = json.loads(r["files"] or "[]")
        out.append({
            "id": r["id"],
            "created_at": r["created_at"],
            "mode": r["mode"],
            "prompt": r["prompt"],
            "model": r["model"],
            "size": r["size"],
            "quality": r["quality"],
            "n": r["n"],
            "files": files,
            "images": [{"filename": f, "url": f"/outputs/{f}"} for f in files],
        })
    return out


def delete_history(item_id: int) -> list:
    """删除一条历史,返回它关联的文件名(供调用方决定是否删盘上文件)。"""
    with _lock:
        row = _db().execute("SELECT files FROM history WHERE id=?", (item_id,)).fetchone()
        files = json.loads(row["files"]) if row and row["files"] else []
        _db().execute("DELETE FROM history WHERE id=?", (item_id,))
        _db().commit()
    return files


# ----------------------------- 收藏 -----------------------------

def add_favorite(prompt: str, name: str = "") -> int:
    with _lock:
        cur = _db().execute(
            "INSERT INTO favorites (created_at, name, prompt) VALUES (?,?,?)",
            (int(time.time()), name or "", prompt),
        )
        _db().commit()
        return cur.lastrowid


def list_favorites() -> list:
    with _lock:
        rows = _db().execute("SELECT * FROM favorites ORDER BY id DESC").fetchall()
    return [{"id": r["id"], "name": r["name"], "prompt": r["prompt"], "created_at": r["created_at"]}
            for r in rows]


def delete_favorite(fav_id: int):
    with _lock:
        _db().execute("DELETE FROM favorites WHERE id=?", (fav_id,))
        _db().commit()
