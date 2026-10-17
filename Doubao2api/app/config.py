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


def save_cookie(cookie: str, path: str = None) -> str:
    """把 DOUBAO_COOKIE 写回 .env 并更新当前进程环境变量，返回写入路径。

    只改这一行、其余原样保留；.env 不在版本控制内（见 .gitignore），凭据不落日志。
    """
    path = path or os.path.join(BASE, ".env")
    line = 'DOUBAO_COOKIE="%s"' % cookie
    lines = []
    if os.path.exists(path):
        with open(path, encoding="utf-8") as f:
            lines = f.read().splitlines()
    for i, old in enumerate(lines):
        if old.strip().startswith("DOUBAO_COOKIE"):
            lines[i] = line
            break
    else:
        if lines and lines[-1].strip():
            lines.append("")
        lines.append("# ---- 上游凭证（/login 扫码写回，也可手工粘贴） ----")
        lines.append(line)
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    os.environ["DOUBAO_COOKIE"] = cookie
    return path
