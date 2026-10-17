"""app/routes/login_qr.py — 扫码登录（/login 网页 + /login/qr JSON 接口）。

为什么放在网关里而不是只写脚本：扫码拿到的 cookie 就是上游凭证，落盘（config.save_cookie）
与热重建客户端（gateway.reload_client）都在这里闭环，不需要重启服务、也不用手工粘贴。
cookie 值只进 .env 与 os.environ，**不进日志、不进响应体**。
"""
from __future__ import annotations

import logging
import threading
import time

from flask import Blueprint, Response, jsonify

from app import config, gateway, qrlogin

bp = Blueprint("login_qr", __name__)
log = logging.getLogger("doubao2api")

# token -> 会话。扫码期间才存在，确认/过期/超时即回收，进程重启即清空。
_SESSIONS: dict[str, qrlogin.QrLoginSession] = {}
_LOCK = threading.Lock()
_SESSION_TTL = 10 * 60


def _prune(now: float = None) -> None:
    """清掉超时会话（调用方需持 _LOCK）。"""
    now = now or time.time()
    for tok, s in list(_SESSIONS.items()):
        if now - s.created_at > _SESSION_TTL:
            _SESSIONS.pop(tok, None)


@bp.post("/login/qr")
def start_login():
    """新建扫码会话：出码并返回二维码。"""
    denied = gateway.guard()
    if denied:
        return denied
    session = qrlogin.QrLoginSession()
    try:
        info = session.start()
    except qrlogin.doubao.DoubaoError as exc:
        return gateway.openai_error(str(exc), "authentication_error", status=502)
    session.created_at = time.time()
    with _LOCK:
        _prune()
        _SESSIONS[session.token] = session
    info.pop("qrcode", None)          # base64 原串没用，data URI 才是给前端的
    info["poll"] = "/login/qr/%s" % session.token
    return jsonify(info)


@bp.get("/login/qr/<token>")
def poll_login(token):
    """轮询扫码状态；confirmed 时收票落盘并热重建上游客户端。"""
    denied = gateway.guard()
    if denied:
        return denied
    with _LOCK:
        session = _SESSIONS.get(token)
    if session is None:
        return jsonify({"error": "会话不存在或已回收，请重新出码"}), 404
    try:
        state = session.poll()
    except qrlogin.doubao.DoubaoError as exc:
        return gateway.openai_error(str(exc), "authentication_error", status=502)

    if not state["confirmed"]:
        if state["status"] == "expired":
            with _LOCK:
                _SESSIONS.pop(token, None)
        return jsonify(state)

    try:
        cookie = session.collect_cookie()
        path = config.save_cookie(cookie)
    except qrlogin.doubao.DoubaoError as exc:
        return gateway.openai_error(str(exc), "authentication_error", status=502)
    with _LOCK:
        _SESSIONS.pop(token, None)

    ok = gateway.reload_client()
    log.info("扫码登录成功，cookie 已写入 %s，上游客户端重建=%s", path, ok)
    return jsonify({"status": "confirmed", "confirmed": True,
                    "cookie_saved": path, "cookie_names": list(qrlogin.doubao.REQUIRED_COOKIES),
                    "client_ready": ok})


_PAGE = """<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>豆包扫码登录 · Doubao2api</title>
<style>
  :root { color-scheme: light dark; }
  body { margin:0; min-height:100vh; display:grid; place-items:center;
         font:15px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif;
         background:#0f1115; color:#e8eaed; }
  .card { width:min(92vw,420px); padding:28px; border-radius:16px; text-align:center;
          background:#171a21; border:1px solid #262b36; }
  h1 { font-size:17px; margin:0 0 4px; font-weight:600; }
  p  { margin:0; color:#9aa4b2; font-size:13px; }
  .qr { width:200px; height:200px; margin:20px auto; padding:10px; border-radius:12px;
        background:#fff; display:grid; place-items:center; }
  .qr img { width:100%; height:100%; display:block; }
  .qr.empty { background:#20242d; color:#9aa4b2; font-size:12px; }
  .state { font-size:14px; min-height:22px; }
  .ok { color:#3ddc84; } .err { color:#ff6b6b; }
  button { margin-top:14px; padding:9px 18px; border-radius:9px; border:1px solid #2f3542;
           background:#222733; color:#e8eaed; font-size:13px; cursor:pointer; }
  code { font-size:12px; color:#9aa4b2; }
</style>
<div class="card">
  <h1>豆包扫码登录</h1>
  <p>用豆包 App 扫一扫，确认后 cookie 自动写入 .env</p>
  <div class="qr empty" id="qr">正在获取二维码…</div>
  <div class="state" id="state"></div>
  <button id="again" hidden>重新出码</button>
  <p style="margin-top:12px"><code id="hint"></code></p>
</div>
<script>
const key = new URLSearchParams(location.search).get("key") || "";
const hdrs = key ? {Authorization: "Bearer " + key} : {};
const qr = document.getElementById("qr"), st = document.getElementById("state");
const again = document.getElementById("again");
document.getElementById("hint").textContent =
  key ? "已带 key 调用 /login/qr" : "未带 key（服务端设了 API_KEY 时需 /login?key=…）";
let timer = null;

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

async function run() {
  clearTimeout(timer); again.hidden = true;
  qr.className = "qr empty"; qr.textContent = "正在获取二维码…"; st.textContent = "";
  let info;
  try {
    const r = await fetch("/login/qr", {method: "POST", headers: hdrs});
    info = await r.json();
    if (!r.ok) throw new Error(info?.error?.message || info?.error || r.status);
  } catch (e) {
    qr.textContent = "出码失败"; st.className = "state err"; st.textContent = String(e);
    again.hidden = false; return;
  }
  qr.className = "qr";
  qr.innerHTML = "";
  const img = new Image(); img.src = info.qrcode_data_uri; img.alt = "登录二维码";
  qr.appendChild(img);
  st.textContent = "等待扫码…";

  const texts = {new: "等待扫码…", scanned: "已扫码，请在手机上确认"};
  for (;;) {
    await sleep(1500);
    let s;
    try {
      const r = await fetch(info.poll, {headers: hdrs});
      s = await r.json();
      if (!r.ok) throw new Error(s?.error?.message || s?.error || r.status);
    } catch (e) {
      st.className = "state err"; st.textContent = "轮询失败：" + e;
      again.hidden = false; return;
    }
    if (s.status === "confirmed") {
      st.className = "state ok";
      st.textContent = "登录成功，cookie 已写入 " + s.cookie_saved +
                       (s.client_ready ? "（上游客户端已就绪）" : "（客户端待重启）");
      qr.className = "qr empty"; qr.textContent = "✔";
      again.hidden = false; return;
    }
    if (s.status === "expired") {
      st.className = "state err"; st.textContent = "二维码已过期";
      again.hidden = false; return;
    }
    st.textContent = texts[s.status] || s.description || s.status;
  }
}
again.onclick = run;
run();
</script>
"""


@bp.get("/login")
def login_page():
    """自包含扫码页（无外部资源、无密钥）。JSON 接口仍走 guard()。"""
    return Response(_PAGE, mimetype="text/html; charset=utf-8")
