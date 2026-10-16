"""离线验证两个协议桥（Anthropic Messages / OpenAI Responses），零真实请求。

用假上游客户端喂固定的 done 帧，检查四条路径：
  /v1/messages      stream=false / true
  /v1/responses     stream=false / true
另外验证流式中途报错时桥层不会把异常漏到响应里。
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

from app import create_app, doubao, gateway  # noqa: E402

PASS, FAIL = [], []


def check(name, cond):
    (PASS if cond else FAIL).append(name)
    print("  [%s] %s" % ("ok  " if cond else "FAIL", name))


DONE = {"text": "你好世界", "brief": "你好世界",
        "conversation_id": "C123", "question_id": "Q1",
        "answer_message_id": "A1", "model_type": "38",
        "suggestions": ["建议一"], "channels": ["chunk_delta"], "log_id": "L1"}


def body_end():
    return {"conversation_id": "C123", "question_id": "Q1",
            "answer_message_id": "A1", "model_type": "38", "log_id": "L1"}


class FakeClient:
    def __init__(self, fail_after=None, kind="risk", code=710022004):
        self.fail_after = fail_after
        self.kind, self.code = kind, code

    def stream(self, text, model, conv):
        def gen():
            yield ("meta", None)
            yield ("delta", "你好")
            if self.fail_after == "early":
                raise doubao.DoubaoError("风控要求验证", self.code, kind=self.kind)
            yield ("delta", "世界")
            yield ("body_end", body_end())
            yield ("done", dict(DONE))
        return gen()


gateway.CLIENT = FakeClient()
app = create_app()
c = app.test_client()

# ------------------------------------------------------------------ /v1/messages
print("=== /v1/messages 非流式 ===")
r = c.post("/v1/messages", json={"model": "doubao-fast",
                                 "messages": [{"role": "user", "content": "hi"}],
                                 "max_tokens": 100})
print("  status=%s" % r.status_code)
if r.status_code == 200:
    j = r.get_json()
    print("  keys=%s" % sorted(j.keys()))
    print("  body=%s" % json.dumps(j, ensure_ascii=False)[:220])
    check("HTTP 200", True)
    check("有 role=assistant", j.get("role") == "assistant")
    check("content 是块数组", isinstance(j.get("content"), list))
    txt = "".join(b.get("text", "") for b in j.get("content") or []
                  if b.get("type") == "text")
    check("正文完整", txt == "你好世界")
    check("带 stop_reason", bool(j.get("stop_reason")))
    check("带 doubao.conversation_id 回写",
          (j.get("doubao") or {}).get("conversation_id") == "C123")
    check("响应头有 X-Doubao-Conversation-Id",
          r.headers.get("X-Doubao-Conversation-Id") == "C123")
else:
    print("  body=%s" % r.get_data(as_text=True)[:300])
    check("非流式 messages 返回 200", False)

print()
print("=== /v1/messages 流式 ===")
r = c.post("/v1/messages", json={"model": "doubao-fast", "stream": True,
                                 "messages": [{"role": "user", "content": "hi"}],
                                 "max_tokens": 100})
sse = r.get_data(as_text=True)
print("  status=%s  事件=%s" % (r.status_code, sorted(set(
    [l.split(":", 1)[1].strip() for l in sse.splitlines()
     if l.startswith("event:")]))))
check("流式 200", r.status_code == 200)
check("Anthropic 流含 message_start", "event: message_start" in sse)
check("Anthropic 流含 content_block_delta", "event: content_block_delta" in sse)
check("Anthropic 流含 message_stop", "event: message_stop" in sse)
check("正文增量都在", ("你好" in sse and "世界" in sse))
check("无异常文本漏进流", "Traceback" not in sse and "application context" not in sse)

# ------------------------------------------------------------------ /v1/responses
print()
print("=== /v1/responses 非流式 ===")
r = c.post("/v1/responses", json={"model": "doubao-fast",
                                  "input": [{"role": "user",
                                             "content": [{"type": "input_text",
                                                          "text": "hi"}]}]})
print("  status=%s" % r.status_code)
if r.status_code == 200:
    j = r.get_json()
    print("  keys=%s" % sorted(j.keys()))
    flat = json.dumps(j, ensure_ascii=False)
    print("  body=%s" % flat[:240])
    check("HTTP 200", True)
    check("object=response", j.get("object") == "response")
    check("正文出现在 output 里", "你好世界" in flat)
    check("status=completed", j.get("status") == "completed")
else:
    print("  body=%s" % r.get_data(as_text=True)[:300])
    check("非流式 responses 返回 200", False)

print()
print("=== /v1/responses 流式 ===")
r = c.post("/v1/responses", json={"model": "doubao-fast", "stream": True,
                                  "input": [{"role": "user",
                                             "content": [{"type": "input_text",
                                                          "text": "hi"}]}]})
sse = r.get_data(as_text=True)
types = sorted(set([json.loads(l[6:]).get("type", "?")
                    for l in sse.splitlines()
                    if l.startswith("data:") and l[5:].strip().startswith("{")]))
print("  status=%s  type=%s" % (r.status_code, types))
check("流式 200", r.status_code == 200)
check("Responses 流含 response.created", "response.created" in sse)
check("Responses 流含 output_text.delta", "response.output_text.delta" in sse)
check("Responses 流含 response.completed", "response.completed" in sse)
check("无异常漏进流", "Traceback" not in sse and "application context" not in sse)

# ------------------------------------------------------------------ 流式错误路径
print()
print("=== 两个桥的流式错误路径（中途风控错误）===")
gateway.CLIENT = FakeClient(fail_after="early")
for route, payload in (
    ("/v1/messages", {"model": "doubao-fast", "stream": True,
                      "messages": [{"role": "user", "content": "hi"}],
                      "max_tokens": 100}),
    ("/v1/responses", {"model": "doubao-fast", "stream": True,
                       "input": [{"role": "user",
                                  "content": [{"type": "input_text",
                                               "text": "hi"}]}]}),
):
    r = app.test_client().post(route, json=payload)
    sse = r.get_data(as_text=True)
    bad = ("application context" in sse) or ("Traceback" in sse)
    # 只统计 delta 事件里的重复：
    # Responses 规范的 output_item.done / Anthropic 的 message_delta 本就允许携带
    # 已发送内容（不是重复下发），所以整串 count 会误报。
    delta_hits = 0
    for line in sse.splitlines():
        if not line.startswith("data:"):
            continue
        seg = line[5:]
        if ("output_text.delta" in seg or '"type":"text_delta"' in seg) \
                and "你好" in seg:
            delta_hits += 1
    print("  %s -> status=%s  len=%d  delta内'你好'出现%d次"
          % (route, r.status_code, len(sse), delta_hits))
    print("     末 180B: %s" % sse[-180:].replace("\n", " | "))
    check("%s 错误路径不崩" % route, not bad)
    check("%s 错误后 delta 未重复下发" % route, delta_hits <= 1)

print()
print("=" * 52)
print("PASS %d  FAIL %d" % (len(PASS), len(FAIL)))
for n in FAIL:
    print("   -", n)
sys.exit(1 if FAIL else 0)
