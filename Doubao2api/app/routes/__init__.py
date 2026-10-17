"""app/routes — 协议路由注册。"""
from __future__ import annotations

from flask import Flask

from app.routes.anthropic_messages import bp as anthropic_bp
from app.routes.login_qr import bp as login_bp
from app.routes.openai_chat import bp as chat_bp
from app.routes.openai_models import bp as models_bp
from app.routes.openai_responses import bp as responses_bp


def register_routes(app: Flask) -> None:
    app.register_blueprint(chat_bp)
    app.register_blueprint(anthropic_bp)
    app.register_blueprint(responses_bp)
    app.register_blueprint(models_bp)
    app.register_blueprint(login_bp)
