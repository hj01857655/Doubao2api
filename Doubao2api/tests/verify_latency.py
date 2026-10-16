"""流式时延验证：首个正文片段、finish_reason、[DONE] 各自的到达时刻。

期望（本轮修复的目标）：finish_reason 与末片正文几乎同时，不再拖到建议阶段结束。
"""
import json
import time
import urllib.request

URL = "http://127.0.0.1:8378/v1/chat/completions"
payload = {"model": "doubao-fast", "stream": True,
           "messages": [{"role": "user", "content": "用一句话介绍长城。"}]}
req = urllib.request.Request(URL, data=json.dumps(payload).encode("utf-8"),
                             headers={"Content-Type": "application/json"},
                             method="POST")
t0 = time.time()
r = urllib.request.urlopen(req, timeout=120)
marks = {}
parts = []
conv = None
for raw in r:
    line = raw.decode("utf-8", "replace").rstrip("\r\n")
    dt = round(time.time() - t0, 2)
    if not line.startswith("data: "):
        continue
    body = line[6:]
    if body == "[DONE]":
        marks.setdefault("done", dt)
        break
    d = json.loads(body)
    if "error" in d:
        print("ERROR:", json.dumps(d["error"], ensure_ascii=False))
        break
    ch = (d.get("choices") or [{}])[0] if d.get("choices") else {}
    piece = (ch.get("delta") or {}).get("content") if ch else None
    if piece:
        parts.append(piece)
        marks.setdefault("first_delta", dt)
        marks["last_delta"] = dt
    if ch and ch.get("finish_reason"):
        marks.setdefault("finish_reason", dt)
    db = d.get("doubao") or {}
    if db.get("conversation_id"):
        conv = db["conversation_id"]

text = "".join(parts)
print("正文 %d 字: %s" % (len(text), text[:60]))
print("conversation_id:", conv)
for k in ("first_delta", "last_delta", "finish_reason", "done"):
    if k in marks:
        print("  %-14s %.2fs" % (k, marks[k]))
if "finish_reason" in marks and "last_delta" in marks:
    gap = marks["finish_reason"] - marks["last_delta"]
    print("  finish_reason 落后末片 %.2fs" % gap)
    print("  [ok ] 提前收尾生效(<1s)" if gap < 1.0
          else "  [WARN] finish_reason 仍拖 %.2fs" % gap)
tail = marks.get("done", 0) - marks.get("finish_reason", 0)
print("  [ok ] finish_reason 之后仍把建议读完再 [DONE] (差 %.2fs)" % tail
      if tail >= 0 else "  [WARN] 收尾异常")
