"""app — Doubao2api 应用包。

分层（对齐社区 doubao2api 惯例）：
  config   .env / 日志 / 鉴权配置
  gateway  数据面：上游客户端、重试、会话缓存、Chat 形状装配
  routes   协议路由（openai_chat / anthropic_messages / openai_responses）
  bridges  协议翻译（Anthropic Messages、OpenAI Responses ⇄ Chat）
"""
from __future__ import annotations

from app import config as _config

_config.load_env()  # 保证任何入口（含测试直连 import）都先装好环境变量


def create_app():
    """装配 Flask 应用并注册全部路由。"""
    from flask import Flask

    from app import doubao, gateway
    from app.routes import register_routes

    app = Flask(__name__)
    register_routes(app)

    @app.get("/health")
    def health():
        from flask import jsonify
        return jsonify({"ok": gateway.CLIENT is not None,
                        "models": list(doubao.MODES),
                        "upstream": "https://%s%s" % (doubao.HOST,
                                                      doubao.COMPLETION_PATH)})

    return app
