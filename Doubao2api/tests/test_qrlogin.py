"""tests/test_qrlogin.py — 扫码登录状态机（离线：只替换 `qrlogin._http_get` 网络边界）。

不联网也能覆盖真实契约：三段端点响应都用**实测抓到的字段名**（error_code / token /
status / redirect_url），所以字段名写错、CSRF 头漏带、cookie 收不全都会在这里炸。
"""
from __future__ import annotations

import json

import pytest

from app import doubao, qrlogin

CSRF_BODY = {"data": {"passport_csrf_token": "csrf-1"}, "message": "success"}
QR_BODY = {"data": {"error_code": 0, "token": "TK-1", "qrcode": "UE5H",
                    "qrcode_index_url": "https://www.doubao.com/qr/TK-1",
                    "expire_time": 1792144451, "app_name": "doubao",
                    "web_name": "豆包"}, "message": "success"}


def _j(obj) -> bytes:
    return json.dumps(obj).encode()


class FakeTransport:
    """按 URL 片段匹配的假传输；值为 list 时按序弹出（用于轮询状态迁移）。"""

    def __init__(self, routes):
        self.routes = routes
        self.calls = []

    def __call__(self, url, headers, cookies, timeout=15.0):
        self.calls.append((url, dict(headers), dict(cookies)))
        for frag, resp in self.routes:
            if frag in url:
                return resp.pop(0) if isinstance(resp, list) else resp
        raise AssertionError("未预期的请求: %s" % url)

    def urls(self):
        return [c[0] for c in self.calls]

    def headers_for(self, frag):
        for url, headers, _ in self.calls:
            if frag in url:
                return headers
        raise AssertionError("没有命中 %s 的调用" % frag)


AUTH_COOKIES = ["sessionid=s-1", "sessionid_ss=s-2", "sid_tt=s-3", "sid_guard=s-4",
                "uid_tt=s-5", "uid_tt_ss=s-6", "d_ticket=s-7", "ttwid=s-8",
                "s_v_web_id=s-9", "passport_csrf_token=csrf-1"]


@pytest.fixture
def routed(monkeypatch):
    """装好「新码 → 已扫 → 已确认 → 回跳收票」的完整链路。"""

    def build():
        routes = [
            ("/passport/safe/csrf_token/", (200, _j(CSRF_BODY), [], "")),
            ("/passport/web/get_qrcode/", (200, _j(QR_BODY), [], "")),
            ("/passport/web/check_qrconnect/", [
                (200, _j({"data": {"error_code": 0, "status": "new"},
                          "message": "success"}), [], ""),
                (200, _j({"data": {"error_code": 0, "status": "scanned"},
                          "message": "success"}), [], ""),
                (200, _j({"data": {"error_code": 0, "status": "confirmed",
                                   "redirect_url": "https://www.doubao.com/landing"},
                          "message": "success"}), [], ""),
            ]),
            ("/landing", (302, b"", AUTH_COOKIES[:4],
                          "https://www.doubao.com/final")),
            ("/final", (200, b"", AUTH_COOKIES[4:], "")),
        ]
        fake = FakeTransport(routes)
        monkeypatch.setattr(qrlogin, "_http_get", fake)
        return fake

    return build


def test_full_flow_confirms_and_collects_cookie(routed):
    fake = routed()
    session = qrlogin.QrLoginSession()

    info = session.start()
    assert info["token"] == "TK-1"
    assert info["qrcode_data_uri"].startswith("data:image/png;base64,UE5H")
    assert info["app_name"] == "doubao"

    assert session.poll()["status"] == "new"
    assert session.poll()["status"] == "scanned"
    confirmed = session.poll()
    assert confirmed["confirmed"] is True

    assert session.collect_cookie() == "; ".join(
        "%s=%s" % (k, v) for k, v in zip(doubao.REQUIRED_COOKIES,
                                         [c.split("=", 1)[1] for c in AUTH_COOKIES]))


def test_csrf_header_only_on_qr_endpoints(routed):
    fake = routed()
    session = qrlogin.QrLoginSession()
    session.start()
    assert "x-tt-passport-csrf-token" not in fake.headers_for("/passport/safe/csrf_token/")
    assert fake.headers_for("/passport/web/get_qrcode/")["x-tt-passport-csrf-token"] == "csrf-1"
    session.poll()
    assert fake.headers_for("/passport/web/check_qrconnect/")["x-tt-passport-csrf-token"] == "csrf-1"


def test_next_param_present_on_qr_and_poll(routed):
    fake = routed()
    session = qrlogin.QrLoginSession()
    session.start()
    session.poll()
    qr_url = [u for u in fake.urls() if "get_qrcode" in u][0]
    assert "next=https%3A%2F%2Fwww.doubao.com" in qr_url
    assert "aid=497858" in qr_url                      # 登录面 aid，不是 582478


def test_collect_cookie_follows_redirect_chain(routed):
    fake = routed()
    session = qrlogin.QrLoginSession()
    session.start()
    session.poll(), session.poll(), session.poll()
    session.collect_cookie()
    assert [u for u in fake.urls() if "final" in u]   # 第二跳确实走过了


def test_csrf_missing_raises_auth(monkeypatch):
    fake = FakeTransport([("/passport/safe/csrf_token/",
                           (200, _j({"data": {}, "message": "error"}), [], ""))])
    monkeypatch.setattr(qrlogin, "_http_get", fake)
    with pytest.raises(doubao.DoubaoError) as e:
        qrlogin.QrLoginSession().start()
    assert e.value.kind == "auth"


def test_qrcode_error_code_raises(routed, monkeypatch):
    monkeypatch.setattr(qrlogin, "_http_get", FakeTransport([
        ("/passport/safe/csrf_token/", (200, _j(CSRF_BODY), [], "")),
        ("/passport/web/get_qrcode/", (200, _j(
            {"data": {"error_code": 22, "description": "非法应用"}}, ), [], "")),
    ]))
    with pytest.raises(doubao.DoubaoError) as e:
        qrlogin.QrLoginSession().start()
    assert "error_code=22" in str(e.value)


def test_poll_reports_expired_from_description(routed, monkeypatch):
    monkeypatch.setattr(qrlogin, "_http_get", FakeTransport([
        ("/passport/safe/csrf_token/", (200, _j(CSRF_BODY), [], "")),
        ("/passport/web/get_qrcode/", (200, _j(QR_BODY), [], "")),
        ("/passport/web/check_qrconnect/", (200, _j(
            {"data": {"error_code": 10001, "description": "二维码已过期"}}, ), [], "")),
    ]))
    session = qrlogin.QrLoginSession()
    session.start()
    assert session.poll()["status"] == "expired"


def test_collect_cookie_requires_confirm():
    with pytest.raises(doubao.DoubaoError):
        qrlogin.QrLoginSession().collect_cookie()
