"""app/routes/openai_models.py — OpenAI 模型列表路由（/v1/models）。"""
from __future__ import annotations

from flask import Blueprint, jsonify

from app import doubao, gateway

bp = Blueprint("openai_models", __name__)


@bp.get("/v1/models")
def list_models():
    """OpenAI 形状的模型清单。

    档位来自本地映射表 `doubao.MODES`，不查上游——因此未配置 cookie 时也可用，
    适合做连通性自检。数据源与 `/health` 的 `models` 字段同一份，两处不会漂移；
    `doubao` 这类别名不单列（`doubao.resolve_model` 仍接受）。
    """
    denied = gateway.guard()
    if denied:
        return denied
    return jsonify({
        "object": "list",
        "data": [
            {"id": name, "object": "model", "owned_by": "doubao", "created": 0}
            for name in doubao.MODES
        ],
    })
