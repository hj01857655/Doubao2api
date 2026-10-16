"""验两件还没证的事：流式输出是否逐块到达、多轮上下文是否由服务端续接。

用法: python verify_stream_multiturn.py <conversation_id>
"""
import json
import sys
import time
import urllib.request

BASE = "http://127.0.0.1:8378/v1/chat/completions"
conv = sys.argv[1] if len(sys.argv) > 1 else ""


def post(payload, headers=None):
    h = {"Content-Type": "application/json"}
    if headers:
        h.update(headers)
    req = urllib.request.Request(BASE, data=json.dumps(payload).encode("utf-8"),
                                headers=h, method="POST")
    return urllib.request.urlopen(req, timeout=120)


print("=== T-A 流式：观察分块是否真的分次到达 ===")
p1 = {"model": "doubao-fast", "stream": True,
      "messages": [{"role": "user",
                    "content": "从1数到12，每个数字单独一行，不要别的字。"}]}
t0 = time.time()
r = post(p1)
chunks = []
arrival = []
cur_event_done = False
for raw in r:
    line = raw.decode("utf-8", "replace").rstrip("\r\n")
    if not line.startswith("data: "):
        continue
    body = line[6:]
    if body == "[DONE]":
        cur_event_done = True
        continue
    d = json.loads(body)
    if "error" in d:
        print("  !! stream error:", json.dumps(d["error"], ensure_ascii=False))
        break
    ch = (d.get("choices") or [{}])[0]
    piece = (ch.get("delta") or {}).get("content")
    if piece:
        chunks.append(piece)
        arrival.append(round(time.time() - t0, 2))
    if ch.get("finish_reason"):
        print("  finish_reason  arrived at %.2fs" % (time.time() - t0))
text = "".join(chunks)
print("  片段数=%d  首个=%.2fs  末个=%.2fs" % (
    len(chunks), arrival[0] if arrival else -1,
    arrival[-1] if arrival else -1))
print("  各片段到达时刻: %s" % (arrival[:14],))
print("  拼接结果: %r" % (text,))
distinct = len(set(arrival)) > 1
print("  [ok ] 正文非空" if text.strip() else "  [FAIL] 正文为空")
print("  [ok ] 确实是分次到达(非一次性)" if distinct else "  [FAIL] 只有一时刻到达")
print("  [ok ] 收到 [DONE]" if cur_event_done else "  [FAIL] 无 [DONE]")
print("  [ok ] 数字 1..12 都在" if all(str(i) in text for i in range(1, 13))
      else "  [WARN] 有缺号: %r" % (text,))

print()
print("=== T-B 多轮：带上次 conversation_id，问它上一轮说了什么 ===")
p2 = {"model": "doubao-fast",
      "messages": [{"role": "user",
                    "content": "我上一句让你做什么？你当时回复了什么？简短回答。"}]}
r2 = post(p2, headers={"X-Doubao-Conversation-Id": conv})
d2 = json.loads(r2.read().decode("utf-8"))
reply = d2["choices"][0]["message"]["content"]
same_conv = d2.get("doubao", {}).get("conversation_id")
print("  传入 conv = %s" % conv)
print("  返回 conv = %s  %s" % (same_conv,
      "一致" if same_conv == conv else "不一致!"))
print("  回复: %r" % (reply,))
hits = [k for k in ("数", "12", "收到") if k in reply]
print("  [ok ] 记住了上下文(提到 %s)" % (hits,) if hits
      else "  [WARN] 回复里没体现上下文，可能 conversation_id 未被服务端续接")
print("  question/answer ids: %s / %s" % (
    d2["doubao"].get("question_id"), d2["doubao"].get("answer_message_id")))
