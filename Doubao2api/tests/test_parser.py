"""离线验证 SSE 解析器与请求面构造，全部用已抓到的真实数据，不打任何真实请求。

fixture_sse.txt = docs/03 那次真实响应的逐帧复刻（仅剥掉签名图片 URL 等无关字段）。
"""
from __future__ import annotations

import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.environ.setdefault("DOUBAO_COOKIE", "; ".join(
    "%s=fake" % k for k in ("sessionid", "sessionid_ss", "sid_tt", "sid_guard",
                            "uid_tt", "uid_tt_ss", "d_ticket", "ttwid",
                            "s_v_web_id", "passport_csrf_token")))

from app import doubao  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
FAKE_COOKIE = "; ".join("%s=fake" % k for k in doubao.REQUIRED_COOKIES)
client = doubao.DoubaoClient(cookie=FAKE_COOKIE)

PASS, FAIL = [], []


def check(name, got, want):
    (PASS if got == want else FAIL).append(name)
    mark = "ok  " if got == want else "FAIL"
    print("  [%s] %s" % (mark, name))
    if got != want:
        print("        got : %r" % (got,))
        print("        want: %r" % (want,))


def feed(lines):
    """把 SSE 文本喂给解析器，收集 (kind, value)。"""
    st = doubao.StreamState()
    end_stages = set()
    cur = {}
    out = []
    events = []
    for line in lines:
        parsed = doubao.parse_sse_line(line, cur)
        if not parsed:
            continue
        ev, data = parsed
        events.append(ev)
        res = client._handle_event(ev, data, st, end_stages)
        if res:
            out.extend(res)
        if "final" in end_stages:
            break
    return out, st, events


# ---------------------------------------------------------------- 1. 真实抓包流
print("=== T1 逐帧复刻 docs/03 的真实响应 ===")
with open(os.path.join(HERE, "fixture_capture.txt"), encoding="utf-8") as f:
    lines = f.read().split("\n")

out, st, events = feed(lines)
deltas = [v for k, v in out if k == "delta"]
body_end = [v for k, v in out if k == "body_end"]
done = [v for k, v in out if k == "done"][0]

check("delta 片段序列", deltas, ["你", "好", "呀！有什么我可以", "帮你的吗？"])
check("拼接正文", done["text"], "你好呀！有什么我可以帮你的吗？")
check("end_type:1 提前发 body_end(可早出 finish_reason)", len(body_end), 1)
check("body_end 带 conversation_id", body_end[0]["conversation_id"], "38447554619021570")
check("用到的正文通道", done["channels"], ["chunk_delta", "stream_chunk_patch1",
                                          "stream_msg_notify"])
check("conversation_id 来自 SSE_ACK", done["conversation_id"], "38447554619021570")
check("question_id(提问) ", done["question_id"], "58756254450320130")
check("answer_message_id 来自 STREAM_MSG_NOTIFY",
      done["answer_message_id"], "58756254450326274")
check("服务端真实 model_type", done["model_type"], "38")
check("建议取最后一帧全量", done["suggestions"],
      ["你能做些什么？", "你是谁？", "你是怎么学习的？"])
check("tts 镜像通道未混入正文", deltas.count("好"), 1)
check("end_type:1 之后未提前关流", "final" in str(events[-1]) or events[-1] == "SSE_REPLY_END", True)

# ---------------------------------------------------------------- 2. 无 CHUNK_DELTA
print()
print("=== T2 探针 P2 形态：完全没有 CHUNK_DELTA，正文只在 patch_object:1 ===")
variant = []
for line in lines:
    if line.startswith("event: CHUNK_DELTA"):
        variant.append(line)          # 保留 event 行占位
        continue
    variant.append(line)
# 真正的无 delta 流：把 delta 帧整块删掉，并把它们的文本并入 patch(1)
nd = []
skip_next_data = False
for line in lines:
    if line.startswith("event: CHUNK_DELTA"):
        skip_next_data = True
        continue
    if skip_next_data and line.startswith("data:"):
        skip_next_data = False
        continue
    nd.append(line)
out2, st2, ev2 = feed(nd)
deltas2 = [v for k, v in out2 if k == "delta"]
done2 = [v for k, v in out2 if k == "done"][0]
check("无 delta 时仍能出正文", deltas2, ["你", "好"])
check("无 delta 时 channels 不含 chunk_delta",
      "chunk_delta" in done2["channels"], False)
check("无 delta 时 text 就是拼到的部分(不被 brief 覆盖)",
      done2["text"], "你好")
check("brief 仍作为独立预览字段保留", done2["brief"], "你好呀！有什么我可以帮你的吗？")

print()
print("=== T2b brief 是预览串，不可当全文（实测缺陷回归防护）===")
# 真实观测：正文 '1\\n2\\n…12' 时 brief 变成 '123456789101112'（换行被吃掉）
mangle_lines = [
    "event: STREAM_MSG_NOTIFY",
    "data: " + json.dumps({"content": {"content_block": [
        {"block_type": 10000, "block_id": "b1",
         "content": {"text_block": {"text": "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"}}}]},
        "meta": {"message_id": "m1"}}, ensure_ascii=False),
    "",
    "event: SSE_REPLY_END",
    "data: " + json.dumps({"end_type": 1, "msg_finish_attr":
                           {"brief": "123456789101112"}}, ensure_ascii=False),
    "",
    "event: SSE_REPLY_END", "data: " + json.dumps({"end_type": 3}), "",
]
outm = [v for k, v in feed(mangle_lines)[0] if k == "done"][0]
check("换行型正文不被 brief 篡改", outm["text"], "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12")
check("brief 与正文不同时空白折叠后可比",
      doubao._norm(outm["brief"]) != outm["text"], True)

# ---------------------------------------------------------------- 3. 错误分支
print()
print("=== T3 错误三分支 ===")


def feed_err(lines_):
    st = doubao.StreamState()
    cur = {}
    for line in lines_:
        parsed = doubao.parse_sse_line(line, cur)
        if not parsed:
            continue
        try:
            client._handle_event(parsed[0], parsed[1], st, set())
        except doubao.DoubaoError as e:
            return e
    return None


risk_payload = json.dumps({
    "error_code": 710022004, "error_msg": "rate limited",
    "extra": {"decision": json.dumps({"code": "10000", "from": "shark_admin",
                                       "type": "verify", "subtype": "slide"})}},
    ensure_ascii=False)
e = feed_err(["event: STREAM_ERROR", "data: " + risk_payload, ""])
check("风控滑块 -> kind=risk", e.kind if e else None, "risk")
check("风控滑块 -> 不可重试", e.retryable if e else None, False)
check("风控滑块 -> 错误信息含 verify/slide",
      ("verify" in str(e) and "slide" in str(e)) if e else False, True)

freq = json.dumps({"error_code": 710022002, "error_msg": "当前服务访问频繁，请稍后重试",
                   "extra": {"ack": "1"}}, ensure_ascii=False)
e = feed_err(["event: STREAM_ERROR", "data: " + freq, ""])
check("过频 -> kind=rate", e.kind if e else None, "rate")
check("过频 -> 可重试", e.retryable if e else None, True)
check("过频 -> 映射 429", e.to_http() if e else None, 429)

e = feed_err(["event: gateway-error",
              "data: " + json.dumps({"code": "Internal",
                                     "message": "ErrorX:code=710012000 message=[user invalid]"}),
              ""])
check("网关身份错误 -> kind=auth", e.kind if e else None, "auth")
check("网关身份错误 -> 映射 401", e.to_http() if e else None, 401)
check("网关身份错误 -> 不可重试", e.retryable if e else None, False)

# ---------------------------------------------------------------- 4. 请求面
print()
print("=== T4 请求面构造 ===")
canon, mode = doubao.resolve_model("doubao-pro")
check("模型解析 doubao-pro -> model_item_key", mode.model_item_key, "5")
check("别名 doubao -> 默认档", doubao.resolve_model("doubao")[1].model_item_key,
      doubao.MODES[doubao.DEFAULT_MODEL].model_item_key)
check("直接传 model_item_key=seed-lite-7b",
      doubao.resolve_model("seed-lite-7b")[1].display, "豆包 2.1 Lite")

q = doubao.build_query()
for must in ("a_bogus", "msToken"):
    check("query 不含 %s（实测不校验，docs/05）" % must, must in q, False)
for must in ("fp=", "tea_uuid=", "device_id=", "web_tab_id=", "chromium_version=",
             "pkg_type=", "tz_name=", "client_platform="):
    check("设备参数齐全 %s" % must, must in q, True)
check("fp == verify_<device_id>", "fp=verify_" + doubao.DEVICE_ID in q, True)
check("tea_uuid == device_id",
      "tea_uuid=" + doubao.DEVICE_ID in q, True)

env = doubao.build_envelope("测试", mode, conversation_id="")
check("首轮 need_create_conversation=True",
      env["option"]["need_create_conversation"], True)
check("首轮 conversation_id 留空", env["client_meta"]["conversation_id"], "")
env2 = doubao.build_envelope("测试", mode, conversation_id="38447554619021570")
check("续轮 need_create_conversation=False",
      env2["option"]["need_create_conversation"], False)
check("续轮带上 conversation_id",
      env2["client_meta"]["conversation_id"], "38447554619021570")
check("conversation_init_ext.model_item_key",
      env["option"]["conversation_init_ext"]["model_item_key"], "5")
check("ext 值全是字符串",
      all(isinstance(v, str) for v in env["ext"].values()), True)
check("块类型 10000=文本", env["messages"][0]["content_block"][0]["block_type"], 10000)

ck = doubao.build_cookie(FAKE_COOKIE)
check("cookie 裁到 10 名", ck.count("=") , 10)
check("cookie 不含 LARK 令牌", "LARK" in ck, False)
try:
    doubao.build_cookie("ttwid=1")
    check("缺必需 cookie 应报错", "no-raise", "raise")
except doubao.DoubaoError as exc:
    check("缺必需 cookie 报错 kind=auth", exc.kind, "auth")

print()
print("=" * 56)
print("PASS %d   FAIL %d" % (len(PASS), len(FAIL)))
if FAIL:
    print("失败项：")
    for n in FAIL:
        print("   -", n)
sys.exit(1 if FAIL else 0)
