"""main.py — 入口（python main.py）。装配与横幅都在这里，应用逻辑在 app/ 包。"""
from __future__ import annotations

import os

from app import config

config.load_env()
config.setup_logging()

from app import create_app, doubao, gateway  # noqa: E402  （须在 load_env 之后）

app = create_app()

if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8378"))
    if gateway.CLIENT is None:
        print("!! 未配置 DOUBAO_COOKIE，服务会以 503 拒绝所有补全请求。")
        print("   在项目根目录建 .env 写入 DOUBAO_COOKIE='...10 个必需 cookie...'")
        print("   必需项：%s" % ", ".join(doubao.REQUIRED_COOKIES))
    print("Doubao2api on http://127.0.0.1:%d  (models: %s)"
          % (port, ", ".join(doubao.MODES)))
    if not config.api_key():
        print("!! 未设 API_KEY/DOUBAO_API_KEY：入站无鉴权（仅本地使用建议绑定 127.0.0.1）")
    app.run(host="127.0.0.1", port=port, threaded=True)
