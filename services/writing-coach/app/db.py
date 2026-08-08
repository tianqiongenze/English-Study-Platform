"""Writing Coach: DB access via esp-db-py (读写分离).

esp-db-py execute_read 返回 list[tuple](普通 cursor,非 DictCursor),
故本模块用显式列顺序解包,不依赖 dict(row)。
"""
from __future__ import annotations
import json
from typing import Any
from esp_db import execute_write, execute_write_returning, execute_read, execute_read_one


async def list_templates(category: str | None = None, limit: int = 20) -> list[dict]:
    if category:
        rows = await execute_read(
            "SELECT id, category, title, structure, difficulty FROM writing_templates "
            "WHERE category=%s ORDER BY difficulty LIMIT %s",
            (category, limit),
        )
    else:
        rows = await execute_read(
            "SELECT id, category, title, structure, difficulty FROM writing_templates "
            "ORDER BY difficulty LIMIT %s",
            (limit,),
        )
    cols = ["id", "category", "title", "structure", "difficulty"]
    return [dict(zip(cols, r)) for r in rows]


async def list_essays(category: str | None = None, limit: int = 20) -> list[dict]:
    if category:
        rows = await execute_read(
            "SELECT id, category, title, content, difficulty, word_count FROM writing_essays "
            "WHERE category=%s ORDER BY difficulty LIMIT %s",
            (category, limit),
        )
    else:
        rows = await execute_read(
            "SELECT id, category, title, content, difficulty, word_count FROM writing_essays "
            "ORDER BY difficulty LIMIT %s",
            (limit,),
        )
    cols = ["id", "category", "title", "content", "difficulty", "word_count"]
    return [dict(zip(cols, r)) for r in rows]


async def save_plan(user_id: int, weak_points: list[str], plan: str) -> int:
    """INSERT writing_plans,返回新行 lastrowid(plan_id 回填)。"""
    return await execute_write_returning(
        "INSERT INTO writing_plans (user_id, weak_points_json, plan_json, status) "
        "VALUES (%s,%s,%s,'active')",
        (user_id, json.dumps(weak_points), plan),
    )


async def get_plan(user_id: int) -> dict | None:
    row = await execute_read_one(
        "SELECT id, user_id, weak_points_json, plan_json, status, created_at "
        "FROM writing_plans WHERE user_id=%s ORDER BY created_at DESC LIMIT 1",
        (user_id,),
    )
    if not row:
        return None
    cols = ["id", "user_id", "weak_points_json", "plan_json", "status", "created_at"]
    d = dict(zip(cols, row))
    d["weak_points"] = json.loads(d.pop("weak_points_json"))
    d["plan"] = d.pop("plan_json")
    # created_at 转 isoformat 字符串(JSON 可序列化)
    ca = d.get("created_at")
    if ca is not None:
        d["created_at"] = ca.isoformat() if hasattr(ca, "isoformat") else str(ca)
    return d


async def save_progress(user_id: int, plan_id: int, task_id: str, completed: bool, score: float) -> int:
    """INSERT writing_progress,返回新行 lastrowid。"""
    return await execute_write_returning(
        "INSERT INTO writing_progress (user_id, plan_id, task_id, completed, score) "
        "VALUES (%s,%s,%s,%s,%s)",
        (user_id, plan_id, task_id, 1 if completed else 0, score),
    )
