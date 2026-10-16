"""错误路径回归测试（离线，零真实请求）。

覆盖两条没被真实联调打到的路径：
  E1 中途失败的退避重试不得把已发出的正文重复吐给客户端
  E2 流式响应生成器里不能调用 jsonify()（Flask 请求上下文此时已弹出）
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


def check(name, got, want):
    (PASS if got == want else FAIL).append(name)
    print("  [%s] %s" % ("ok  " if got == want else "FAIL", name))
    if got != want:
        print("        got : %r\n        want: %r" % (got, want))


def done_frame(conv="C1"):
    return ("done", {"text": "世界", "brief": "世界", "conversation_id": conv,
                     "question_id": "Q1", "answer_message_id": "A1",
                     "model_type": "38", "suggestions": [], "channels": [],
                     "log_id": "L1"})


def body_end_frame(conv="C1"):
    return ("body_end", {"conversation_id": conv, "question_id": "Q1",
                         "answer_message_id": "A1", "model_type": "38",
                         "log_id": "L1"})


# ---------------------------------------------------------------- E1 重试重复
print("=== E1 已发出正文后失败：不得重放 ===")
calls = {"n": 0}


class FlakyClient:
    """第一次吐 2 个 delta 后抛可重试错误；第二次成功。"""

    def stream(self, text, model, conv):
        calls["n"] += 1
        if calls["n"] == 1:
            def gen():
                yield ("meta", None)
                yield ("delta", "你好")
                yield ("delta", "，世界")
                raise doubao.DoubaoError("rate limited", 710022002, kind="rate")
            return gen()
        return iter([("meta", None), ("delta", "你好"), ("delta", "，世界"),
                     body_end_frame(), done_frame()])


gateway.CLIENT = FlakyClient()
calls["n"] = 0
try:
    got = list(gateway.backoff_stream("x", "doubao-fast", ""))
except doubao.DoubaoError:
    got = []
pieces = [v for k, v in got if k == "delta"]
print("  观测：上游被调 %d 次，deltas=%r" % (calls["n"], pieces))

# 期望的正确行为：已经吐过 delta 就不该重试（避免重复），应把错误抛出
gateway.CLIENT = FlakyClient()
calls["n"] = 0
out = []
raised = None
try:
    for k, v in gateway.backoff_stream("x", "doubao-fast", ""):
        out.append((k, v))
except doubao.DoubaoError as exc:
    raised = exc
deltas = [v for k, v in out if k == "delta"]
print("  实际：上游调用 %d 次, deltas=%r, raised=%s" % (calls["n"], deltas, raised))
check("E1 修复后不得出现重复正文", len(deltas) > 2, False)
check("E1 修复后已发正文即不重试", calls["n"], 1)
check("E1 修复后错误被原样抛出", raised is not None, True)

# ---------------------------------------------------------------- E2 jsonify 上下文
print()
print("=== E2 流式错误路径不得在生成器内用 jsonify ===")


class AlwaysFailClient:
    def __init__(self, kind="risk", code=710022004):
        self.kind, self.code = kind, code

    def stream(self, text, model, conv):
        def gen():
            yield ("meta", None)
            yield ("delta", "前半句")
            raise doubao.DoubaoError("风控要求验证 type=verify subtype=slide",
                                     self.code, kind=self.kind)
        return gen()


gateway.CLIENT = AlwaysFailClient()
app = create_app()
client = app.test_client()

body = json.dumps({"model": "doubao-fast", "stream": True,
                   "messages": [{"role": "user", "content": "hi"}]}).encode()
try:
    resp = client.post("/v1/chat/completions", data=body,
                       content_type="application/json")
    status = resp.status_code
    data = resp.get_data(as_text=True)   # 真正驱动生成器
    print("  status=%s  末 260B: %s" % (status, data[-260:].replace("\n", " | ")))
    check("流式请求未 500", status == 200, True)
    check("未抛 application context 错误",
          "Working outside of application context" in data
          or "Working outside of request context" in data, False)
    check("错误以 SSE error 帧呈现", '"error"' in data, True)
    check("已发出的正文未被重复", data.count("前半句"), 1)
    check("末尾有 [DONE]", data.rstrip().endswith("[DONE]"), True)
except Exception as exc:
    print("  EXC: %s: %s" % (type(exc).__name__, exc))
    check("流式请求未抛异常", "raised", "无异常")

print()
print("=" * 52)
print("PASS %d  FAIL %d" % (len(PASS), len(FAIL)))
sys.exit(1 if FAIL else 0)
