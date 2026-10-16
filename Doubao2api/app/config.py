"""app/config.py — .env 加载 / 日志 / 入站鉴权配置（进程级，先于其它 app 模块导入）。"""
from __future__ import annotations

import logging
import os
from logging.handlers import RotatingFileHandler

BASE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))  # 仓库根


def load_env(path: str = None) -> None:
    """极简 .env：KEY=VALUE，# 注释，不覆盖已存在的环境变量。"""
    path = path or os.path.join(BASE, ".env")
    if not os.path.exists(path):
        return
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k, v = k.strip(), v.strip().strip('"').strip("'")
            os.environ.setdefault(k, v)


def setup_logging() -> None:
    logging.basicConfig(level=logging.INFO)
    _h = RotatingFileHandler(os.path.join(BASE, "server.log"),
                             maxBytes=2_000_000, backupCount=3, encoding="utf-8")
    _h.setFormatter(logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s"))
    logging.getLogger("doubao2api").addHandler(_h)


def api_key() -> str:
    """入站鉴权密钥；空串 = 本地裸奔模式。"""
    return os.environ.get("API_KEY") or os.environ.get("DOUBAO_API_KEY") or ""
