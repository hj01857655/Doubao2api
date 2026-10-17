"""app/qrlogin.py — 豆包扫码登录（passport 三端点，全部实测）。

端点、参数与坑都是实测结论（2026-10-17 curl 直连 www.doubao.com），勿凭感觉改：

  GET /passport/safe/csrf_token/?aid=497858
      -> {"data":{"passport_csrf_token":"<32hex>"},"message":"success"}
  GET /passport/web/get_qrcode/?next=<urlencoded>&aid=497858
      + 头 x-tt-passport-csrf-token: <csrf>
      -> error_code=0，token=<35 字符>，qrcode=<base64 PNG>
  GET /passport/web/check_qrconnect/?next=<urlencoded>&token=<token>&aid=497858
      + 同一个 CSRF 头
      -> status: new -> scanned -> confirmed（附 redirect_url）/ expired

三个坑（都实测踩过）：

1. 登录面 aid = 497858，与桌面 chat 面 aid = 582478 **不是**一套。串用只会拿到
   error_code 22「非法应用」，看起来像端点不存在，实际是应用 id 不对。
2. 端点是 **GET + 尾斜杠**，且必须带 `next`（登录后回跳地址）。缺尾斜杠时服务端
   回 301/307，缺 next 时按非法调用处置。
3. CSRF 必须放在 **头** `x-tt-passport-csrf-token`，塞进 query 不生效。

实现只用 stdlib（对齐 app/doubao.py 的零依赖风格）。`_http_get` 是唯一的网络边界，
单元测试替换它即可离线跑完整状态机。
"""
from __future__ import annotations

import http.client
import json
import logging
import ssl
import threading
import urllib.parse

from app import doubao

log = logging.getLogger("doubao2api")

BASE = "https://www.doubao.com"
LOGIN_AID = "497858"
LOGIN_REFERER = BASE + "/chat/login"
# 扫码成功后回跳目标：与登录页同源，服务端据此决定 redirect_url 的落点。
NEXT_URL = BASE
UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/%s Safari/537.36" % doubao.CHROMIUM_VERSION)

# 跟随 redirect_url 的最大跳数：实测登录回跳链为 1~3 跳，6 跳足够且能防打转。
MAX_HOPS = 6


def _headers(csrf: str = "") -> dict:
    h = {
        "User-Agent": UA,
        "Accept": "application/json, text/plain, */*",
        "Referer": LOGIN_REFERER,
        "Origin": BASE,
    }
    if csrf:
        h["x-tt-passport-csrf-token"] = csrf
    return h


def _http_get(url, headers, cookies, timeout=15.0):
    """GET 一次，返回 ``(status, body_bytes, set_cookie 原始串列表, location)``。

    不自动跟随跳转：登录回跳链上的每一跳都可能下发 Set-Cookie，必须逐跳收。
    """
    parts = urllib.parse.urlsplit(url)
    path = parts.path + (("?" + parts.query) if parts.query else "")
    hdrs = dict(headers)
    if cookies:
        hdrs["Cookie"] = "; ".join("%s=%s" % kv for kv in cookies.items())
    conn = http.client.HTTPSConnection(parts.netloc, timeout=timeout,
                                       context=ssl.create_default_context())
    try:
        conn.request("GET", path, headers=hdrs)
        resp = conn.getresponse()
        return (resp.status, resp.read(),
                resp.headers.get_all("Set-Cookie") or [],
                resp.headers.get("Location") or "")
    finally:
        conn.close()


def _merge_cookies(cookies: dict, raw_set_cookies) -> None:
    for raw in raw_set_cookies:
        pair = raw.split(";", 1)[0].strip()
        if "=" in pair:
            k, v = pair.split("=", 1)
            cookies[k.strip()] = v.strip()


def _json(body: bytes) -> dict:
    try:
        return json.loads(body.decode("utf-8", "replace"))
    except ValueError:
        return {}


def _fail(action: str, data: dict, body: bytes) -> doubao.DoubaoError:
    code = (data or {}).get("error_code", -1)
    desc = (data or {}).get("description") or ""
    return doubao.DoubaoError(
        "%s失败: error_code=%s %s %s" % (action, code, desc,
                                         (body[:120].decode("utf-8", "replace"))),
        kind="auth")


def png_data_uri(qrcode_b64: str) -> str:
    """`get_qrcode` 的 `qrcode` 字段是裸 base64 PNG（不是 data URI）。"""
    if qrcode_b64.startswith("data:"):
        return qrcode_b64
    return "data:image/png;base64," + qrcode_b64


class QrLoginSession:
    """一次扫码登录的状态机：``start()`` 出码 → ``poll()`` 轮询 → ``collect_cookie()`` 收票。"""

    def __init__(self, aid: str = LOGIN_AID, next_url: str = NEXT_URL):
        self.aid = aid
        self.next_url = next_url
        self.cookies: dict[str, str] = {}
        self.token = ""
        self.qrcode = ""
        self.qrcode_index_url = ""
        self.expire_time = 0
        self.status = "idle"
        self.description = ""
        self.redirect_url = ""
        self.created_at = 0.0
        self._lock = threading.Lock()

    # ---------------------------------------------------------------- 步骤 1
    def csrf_token(self) -> str:
        """取（并缓存）passport CSRF。响应里没有 token 就是被风控拦了。"""
        if self.cookies.get("passport_csrf_token"):
            return self.cookies["passport_csrf_token"]
        status, body, sc, _ = _http_get(
            "%s/passport/safe/csrf_token/?aid=%s" % (BASE, self.aid),
            _headers(), self.cookies)
        _merge_cookies(self.cookies, sc)
        data = _json(body)
        token = (data.get("data") or {}).get("passport_csrf_token", "")
        if not token:
            raise _fail("获取 CSRF", data.get("data") or data, body)
        with self._lock:
            self.cookies["passport_csrf_token"] = token
        return token

    # ---------------------------------------------------------------- 步骤 2
    def start(self) -> dict:
        """取二维码。`next` 必须带，否则服务端按非法调用处置。"""
        csrf = self.csrf_token()
        url = "%s/passport/web/get_qrcode/?next=%s&aid=%s" % (
            BASE, urllib.parse.quote(self.next_url, safe=""), self.aid)
        _, body, sc, _ = _http_get(url, _headers(csrf), self.cookies)
        _merge_cookies(self.cookies, sc)
        data = _json(body)
        d = data.get("data") or {}
        if d.get("error_code", data.get("error_code", -1)) != 0 or not d.get("token"):
            raise _fail("获取二维码", d, body)
        with self._lock:
            self.token = d["token"]
            self.qrcode = d.get("qrcode", "")
            self.qrcode_index_url = d.get("qrcode_index_url", "")
            self.expire_time = d.get("expire_time", 0) or 0
            self.status = "new"
        log.info("扫码会话已创建 token=%s… expire_time=%s",
                 self.token[:8], self.expire_time)
        return {
            "token": self.token,
            "qrcode": self.qrcode,
            "qrcode_data_uri": png_data_uri(self.qrcode) if self.qrcode else "",
            "qrcode_index_url": self.qrcode_index_url,
            "expire_time": self.expire_time,
            "app_name": d.get("app_name", ""),
            "web_name": d.get("web_name", ""),
            "status": self.status,
        }

    # ---------------------------------------------------------------- 步骤 3
    def poll(self) -> dict:
        """查一次扫码状态；服务端不区分「过期」错误码，靠 description 判定。"""
        if not self.token:
            raise doubao.DoubaoError("尚未 start()，没有二维码 token", kind="auth")
        url = "%s/passport/web/check_qrconnect/?next=%s&token=%s&aid=%s" % (
            BASE, urllib.parse.quote(self.next_url, safe=""), self.token, self.aid)
        csrf = self.cookies.get("passport_csrf_token", "")
        _, body, sc, _ = _http_get(url, _headers(csrf), self.cookies)
        _merge_cookies(self.cookies, sc)
        data = _json(body)
        d = data.get("data") or {}
        code = d.get("error_code", data.get("error_code", -1))
        with self._lock:
            self.description = d.get("description") or data.get("message") or ""
            if code != 0:
                if "expired" in self.description.lower() or "过期" in self.description:
                    self.status = "expired"
                return self._state(code)
            self.status = d.get("status") or self.status
            self.redirect_url = d.get("redirect_url") or self.redirect_url
            return self._state(0)

    def _state(self, code: int) -> dict:
        return {
            "token": self.token,
            "status": self.status,
            "description": self.description,
            "error_code": code,
            "expire_time": self.expire_time,
            "confirmed": self.status == "confirmed",
        }

    # ---------------------------------------------------------------- 步骤 4
    def collect_cookie(self) -> str:
        """confirmed 之后跟随 redirect_url 收票，返回可写盘的 cookie 串。

        `sessionid` 等 10 个鉴权名是在这条回跳链的 Set-Cookie 上下发的，所以必须
        逐跳收；只取 `doubao.REQUIRED_COOKIES` 里缺一不可的那份，顺带完成校验。
        """
        if not self.redirect_url:
            raise doubao.DoubaoError("未收到 redirect_url，登录尚未确认", kind="auth")
        url = self.redirect_url
        for _ in range(MAX_HOPS):
            status, _, sc, location = _http_get(url, _headers(), self.cookies)
            _merge_cookies(self.cookies, sc)
            if status not in (301, 302, 303, 307, 308) or not location:
                break
            url = urllib.parse.urljoin(url, location)
        missing = [k for k in doubao.REQUIRED_COOKIES if not self.cookies.get(k)]
        if missing:
            raise doubao.DoubaoError(
                "登录回跳后 cookie 仍缺: %s" % ", ".join(missing), kind="auth")
        return "; ".join("%s=%s" % (k, self.cookies[k])
                         for k in doubao.REQUIRED_COOKIES)
