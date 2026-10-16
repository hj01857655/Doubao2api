# -*- coding: utf-8 -*-
"""anthropic_bridge.py — Anthropic Messages ⇄ OpenAI Chat Completions 协议桥

与 responses_bridge.py 同构的中间翻译层。背景：Claude Code / Kiro 这类
客户端原生只说 Anthropic Messages 协议（POST /v1/messages，SSE 每帧
`event: <type>\\ndata: <json>\\n\\n`）。上游免费层只出 Chat Completions，
且 /inference/v1/messages 是付费凭证面（免费模型 403 FreeTierError、付费
模型 401 Missing API key，2026-10-15 实测），走不了直通。所以本模块在本地
把协议翻掉，上游照旧走 chat 线。

对外三个口子（与 responses_bridge 对齐）：
  请求侧  anthropic_to_chat(body)            Anthropic 请求体 -> Chat 请求体
  响应侧  chat_sse_to_anthropic(stream, m)    Chat SSE 字节流 -> Anthropic SSE
          chat_json_to_anthropic(chat, m)     非流式 Chat JSON -> Anthropic JSON
          aggregate_chat_sse(stream, m)       Chat SSE -> 单个 Chat JSON（非流式聚合）

闸门工具（bash/glob/grep/read 小写）不在这里注入，留给各项目 server.py 的
路由按既有 chat 线做法补 —— 本模块只翻协议，不碰指纹/鉴权。

工具名大小写：Claude Code 发 PascalCase（Bash/Read/...），闸门要小写。与
chat 线一致：客户端工具原名保留、小写 stub 追加在后面，回程不改名。模型
极少去调 stub（描述空、参数单字段）；真调到了与 chat 线同症状，不在 v1 特判。
"""

import json
import time
import uuid


def _hex(n=24):
    return uuid.uuid4().hex[:n]


# ============================================================
# 一、请求侧：Anthropic Messages -> Chat Completions
# ============================================================

def _image_block_to_chat(b):
    """Anthropic image block -> Chat image_url block；转不了返回 None。"""
    src = b.get("source") if isinstance(b, dict) else None
    if not isinstance(src, dict):
        return None
    if src.get("type") == "base64":
        mt = src.get("media_type") or "image/png"
        data = src.get("data") or ""
        if data:
            return {"type": "image_url",
                    "image_url": {"url": "data:%s;base64,%s" % (mt, data)}}
    if src.get("type") == "url":
        url = src.get("url") or ""
        if url:
            return {"type": "image_url", "image_url": {"url": url}}
    return None


def _plain_content_to_chat(content):
    """Anthropic content（纯文本/图片块，无 tool_use/tool_result）-> Chat content。"""
    if isinstance(content, str):
        return content
    if not isinstance(content, list):
        return str(content or "")
    parts = []
    for b in content:
        if isinstance(b, str):
            parts.append({"type": "text", "text": b})
            continue
        if not isinstance(b, dict):
            continue
        if b.get("type") == "text":
            parts.append({"type": "text", "text": b.get("text") or ""})
        elif b.get("type") == "image":
            img = _image_block_to_chat(b)
            if img is not None:
                parts.append(img)
    if not parts:
        return ""
    if len(parts) == 1 and parts[0]["type"] == "text":
        return parts[0]["text"]
    return parts


def _tool_result_content_to_str(out):
    """tool_result.content（str 或块数组）-> 单个字符串。"""
    if isinstance(out, str):
        return out
    if isinstance(out, list):
        segs = []
        for b in out:
            if isinstance(b, str):
                segs.append(b)
            elif isinstance(b, dict) and b.get("type") == "text":
                segs.append(b.get("text") or "")
        return "".join(segs)
    if out is None:
        return ""
    return json.dumps(out, ensure_ascii=False)


def _messages_to_chat(body):
    """Anthropic system + messages -> Chat messages。

    - system（str 或文本块数组）-> 首条 system
    - assistant 的 tool_use 块 -> assistant.tool_calls（同条文本并入 content）
    - user 的 tool_result 块 -> role=tool（每块一条），剩余文本另起 user
    - 纯文本/图片 -> 普通 user/assistant
    """
    messages = []
    sysc = body.get("system")
    if isinstance(sysc, str) and sysc:
        messages.append({"role": "system", "content": sysc})
    elif isinstance(sysc, list):
        txt = " ".join(b.get("text", "") for b in sysc
                       if isinstance(b, dict) and b.get("type") == "text")
        if txt:
            messages.append({"role": "system", "content": txt})

    for m in body.get("messages") or []:
        if not isinstance(m, dict):
            continue
        role = m.get("role") or "user"
        if role not in ("user", "assistant"):
            role = "user"
        content = m.get("content")
        if isinstance(content, str) or not isinstance(content, list):
            messages.append({"role": role,
                             "content": content if isinstance(content, str)
                             else str(content or "")})
            continue

        tool_uses = [b for b in content
                     if isinstance(b, dict) and b.get("type") == "tool_use"]
        tool_results = [b for b in content
                        if isinstance(b, dict) and b.get("type") == "tool_result"]
        text_txt = " ".join(
            (b.get("text", "") if isinstance(b, dict) else str(b))
            for b in content
            if (isinstance(b, dict) and b.get("type") == "text")
            or isinstance(b, str))

        if tool_results:
            # user 回工具结果：每条 tool_result -> role=tool；余下文本另起 user
            for tr in tool_results:
                messages.append({
                "role": "tool",
                "tool_call_id": tr.get("tool_use_id") or "",
                "content": _tool_result_content_to_str(tr.get("content")),
                })
            if text_txt:
                messages.append({"role": "user", "content": text_txt})
        elif tool_uses:
            tc_list = []
            for tu in tool_uses:
                tc_list.append({
                    "id": tu.get("id") or "call_" + _hex(),
                    "type": "function",
                    "function": {
                        "name": tu.get("name") or "",
                        "arguments": json.dumps(tu.get("input") or {},
                                                 ensure_ascii=False),
                    },
                })
            messages.append({"role": "assistant",
                             "content": text_txt or None,
                             "tool_calls": tc_list})
        else:
            messages.append({"role": role,
                             "content": _plain_content_to_chat(content)})
    return messages


def _tools_to_chat(tools):
    """Anthropic tools（input_schema）-> Chat tools（function.parameters）。"""
    if not isinstance(tools, list):
        return None
    out = []
    for t in tools:
        if not isinstance(t, dict):
            continue
        out.append({
            "type": "function",
            "function": {
                "name": t.get("name") or "",
                "description": t.get("description") or "",
                "parameters": t.get("input_schema")
                or {"type": "object", "properties": {}},
            },
        })
    return out or None


def _tool_choice_to_chat(tc):
    if tc is None:
        return None
    if isinstance(tc, dict):
        tt = tc.get("type")
        if tt == "auto":
            return "auto"
        if tt == "any":
            return "required"
        if tt == "none":
            return "none"
        if tt == "tool":
            return {"type": "function", "function": {"name": tc.get("name") or ""}}
    return None


def anthropic_to_chat(body):
    """Anthropic Messages 请求体 -> Chat Completions 请求体。

    只搬 Chat 能表达的字段；thinking / metadata / top_k 等无对应项丢弃（v1）。
    """
    chat = {
        "model": body.get("model") or "",
        "messages": _messages_to_chat(body),
        "stream": bool(body.get("stream", False)),
    }
    tools = _tools_to_chat(body.get("tools"))
    if tools:
        chat["tools"] = tools
    tc = _tool_choice_to_chat(body.get("tool_choice"))
    if tc is not None:
        chat["tool_choice"] = tc
    if body.get("max_tokens") is not None:
        chat["max_tokens"] = body["max_tokens"]
    if body.get("temperature") is not None:
        chat["temperature"] = body["temperature"]
    if body.get("top_p") is not None:
        chat["top_p"] = body["top_p"]
    if isinstance(body.get("stop_sequences"), list) and body["stop_sequences"]:
        chat["stop"] = body["stop_sequences"]
    return chat


# ============================================================
# 二、响应侧公共
# ============================================================

def _stop_reason(finish):
    """Chat finish_reason -> Anthropic stop_reason。"""
    return {
        "stop": "end_turn",
        "tool_calls": "tool_use",
        "length": "max_tokens",
    }.get(finish, "end_turn")


def _anthropic_usage(u):
    """Chat usage -> Anthropic usage（四个字段齐全）。"""
    if not isinstance(u, dict):
        u = {}
    return {
        "input_tokens": int(u.get("prompt_tokens") or 0),
        "output_tokens": int(u.get("completion_tokens") or 0),
        "cache_creation_input_tokens": 0,
        "cache_read_input_tokens": 0,
    }


def err_type_for_http(status):
    """HTTP 状态码 -> Anthropic error type。"""
    return {
        400: "invalid_request_error",
        401: "authentication_error",
        403: "permission_error",
        404: "not_found_error",
        429: "rate_limit_error",
        529: "overloaded_error",
    }.get(status, "api_error")


def error_body(err_type, message):
    """Anthropic 错误响应体。"""
    return {"type": "error",
            "error": {"type": err_type, "message": message or "error"}}


# ============================================================
# 三、非流式：Chat JSON -> Anthropic Message
# ============================================================

def chat_json_to_anthropic(chat, model=None):
    """非流式 Chat Completions JSON -> Anthropic Message 对象（dict）。"""
    model = model or chat.get("model") or ""
    msg = {}
    finish = "stop"
    choices = chat.get("choices")
    if isinstance(choices, list) and choices and isinstance(choices[0], dict):
        m = choices[0].get("message")
        if isinstance(m, dict):
            msg = m
        fr = choices[0].get("finish_reason")
        if fr:
            finish = fr

    content_blocks = []
    text = msg.get("content")
    if isinstance(text, str) and text:
        content_blocks.append({"type": "text", "text": text})
    tcs = msg.get("tool_calls")
    if isinstance(tcs, list):
        for tc in tcs:
            if not isinstance(tc, dict):
                continue
            fn = tc.get("function") or {}
            args = fn.get("arguments") or ""
            try:
                inp = json.loads(args) if args else {}
            except (json.JSONDecodeError, ValueError):
                inp = {}
            content_blocks.append({
                "type": "tool_use",
                "id": tc.get("id") or "toolu_" + _hex(),
                "name": fn.get("name") or "",
                "input": inp,
            })

    return {
        "id": "msg_" + _hex(24),
        "type": "message",
        "role": "assistant",
        "model": model,
        "content": content_blocks,
        "stop_reason": _stop_reason(finish),
        "stop_sequence": None,
        "usage": _anthropic_usage(chat.get("usage")),
    }


# ============================================================
# 四、流式：Chat SSE -> Anthropic SSE
# ============================================================

def _ant_frame(event_type, data):
    """组装一帧 `event: <type>\\ndata: <json>\\n\\n`。"""
    d = dict(data)
    d["type"] = event_type
    return (b"event: " + event_type.encode("utf-8") + b"\ndata: "
            + json.dumps(d, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            + b"\n\n")


def chat_sse_to_anthropic(stream, model):
    """Chat SSE 原始行流 -> Anthropic SSE 帧流（generator）。

    stream: 迭代器，每项是原始行（bytes 或 str，带 "data: " 前缀）。
    产出: 每项一个完整帧 b"event: ...\\ndata: ...\\n\\n"。
    收尾: content_block_stop * N -> message_delta -> message_stop；
          上游错误帧 -> error 事件后结束。
    """
    msg_id = "msg_" + _hex(24)
    base_usage = {"input_tokens": 0, "output_tokens": 0,
                  "cache_creation_input_tokens": 0,
                  "cache_read_input_tokens": 0}
    blocks = []          # 已开出的 content block（按序）
    text_idx = None      # 文本块在 blocks 中的下标
    tool_blocks = {}     # chat tool_calls.index -> blocks 下标
    finish = None
    usage = None
    upstream_error = None

    yield _ant_frame("message_start", {"message": {
        "id": msg_id, "type": "message", "role": "assistant", "content": [],
        "model": model, "stop_reason": None, "stop_sequence": None,
        "usage": base_usage}})

    try:
        for raw in stream:
            line = (raw.decode("utf-8", "replace")
                    if isinstance(raw, (bytes, bytearray)) else str(raw))
            line = line.strip()
            if not line.startswith("data:"):
                continue
            payload = line[5:].strip()
            if not payload or payload == "[DONE]":
                continue
            try:
                chunk = json.loads(payload)
            except (json.JSONDecodeError, ValueError):
                continue
            if not isinstance(chunk, dict):
                continue

            err = chunk.get("error")
            if isinstance(err, dict) and err.get("message"):
                upstream_error = (str(err.get("code") or "upstream_error"),
                                  str(err["message"]))
                break
            if chunk.get("error_code") or chunk.get("error_msg"):
                upstream_error = (str(chunk.get("error_code") or "upstream_error"),
                                  str(chunk.get("error_msg") or "upstream error"))
                break

            if isinstance(chunk.get("usage"), dict):
                usage = chunk["usage"]

            choices = chunk.get("choices")
            if not (isinstance(choices, list) and choices
                    and isinstance(choices[0], dict)):
                continue
            choice = choices[0]
            fr = choice.get("finish_reason")
            if fr:
                finish = fr
            delta = choice.get("delta")
            if not isinstance(delta, dict):
                continue

            # 1) 文本 delta
            ct = delta.get("content")
            if isinstance(ct, str) and ct:
                if text_idx is None:
                    text_idx = len(blocks)
                    blocks.append("text")
                    yield _ant_frame("content_block_start", {
                        "index": text_idx,
                        "content_block": {"type": "text", "text": ""}})
                yield _ant_frame("content_block_delta", {
                    "index": text_idx,
                    "delta": {"type": "text_delta", "text": ct}})

            # 2) tool_calls delta
            tcs = delta.get("tool_calls")
            if isinstance(tcs, list):
                for tc in tcs:
                    if not isinstance(tc, dict):
                        continue
                    i = tc.get("index")
                    if not isinstance(i, int):
                        i = len(tool_blocks)
                    fn = tc.get("function") or {}
                    bidx = tool_blocks.get(i)
                    if bidx is None:
                        bidx = len(blocks)
                        call_id = tc.get("id") or "toolu_" + _hex()
                        name = fn.get("name") or ""
                        blocks.append("tool_use")
                        tool_blocks[i] = bidx
                        yield _ant_frame("content_block_start", {
                            "index": bidx,
                            "content_block": {"type": "tool_use",
                                              "id": call_id, "name": name,
                                              "input": {}}})
                    args = fn.get("arguments")
                    if isinstance(args, str) and args:
                        yield _ant_frame("content_block_delta", {
                            "index": bidx,
                            "delta": {"type": "input_json_delta",
                                      "partial_json": args}})
    except Exception:
        # 上游断流：不让异常掀掉 SSE，照常收尾
        pass

    if upstream_error is not None:
        _code, msg = upstream_error
        yield _ant_frame("error", {"error": {"type": "api_error",
                                             "message": msg}})
        return

    for idx in range(len(blocks)):
        yield _ant_frame("content_block_stop", {"index": idx})

    out_tokens = int((usage or {}).get("completion_tokens") or 0) if usage else 0
    yield _ant_frame("message_delta", {
        "delta": {"stop_reason": _stop_reason(finish), "stop_sequence": None},
        "usage": {"output_tokens": out_tokens}})
    yield _ant_frame("message_stop", {})


# ============================================================
# 五、非流式聚合：Chat SSE -> 单个 Chat JSON
# ============================================================

def aggregate_chat_sse(stream, model):
    """Chat SSE 行流 -> 单个 chat.completion JSON dict。

    供非流式 Anthropic 路径用：上游恒流式，先聚合成 Chat JSON，再交给
    chat_json_to_anthropic 翻成 Anthropic Message。比 chat 线的简易聚合多收
    tool_calls 与 usage（Anthropic 非流式要这两项）。
    """
    content = []
    tool_calls = {}      # index -> {id,name,arguments}
    finish = None
    usage = None
    upstream_error = None

    for raw in stream:
        line = (raw.decode("utf-8", "replace")
                if isinstance(raw, (bytes, bytearray)) else str(raw))
        line = line.strip()
        if not line.startswith("data:"):
            continue
        payload = line[5:].strip()
        if not payload or payload == "[DONE]":
            continue
        try:
            chunk = json.loads(payload)
        except (json.JSONDecodeError, ValueError):
            continue
        if not isinstance(chunk, dict):
            continue

        err = chunk.get("error")
        if isinstance(err, dict) and err.get("message"):
            upstream_error = (str(err.get("code") or "upstream_error"),
                              str(err["message"]))
            break
        if isinstance(chunk.get("usage"), dict):
            usage = chunk["usage"]

        choices = chunk.get("choices")
        if not (isinstance(choices, list) and choices
                and isinstance(choices[0], dict)):
            continue
        choice = choices[0]
        fr = choice.get("finish_reason")
        if fr:
            finish = fr
        delta = choice.get("delta")
        if not isinstance(delta, dict):
            continue
        if isinstance(delta.get("content"), str) and delta["content"]:
            content.append(delta["content"])
        tcs = delta.get("tool_calls")
        if isinstance(tcs, list):
            for tc in tcs:
                if not isinstance(tc, dict):
                    continue
                i = tc.get("index")
                if not isinstance(i, int):
                    i = len(tool_calls)
                fn = tc.get("function") or {}
                stl = tool_calls.get(i)
                if stl is None:
                    stl = {"id": tc.get("id") or "call_" + _hex(),
                           "name": fn.get("name") or "", "arguments": ""}
                    tool_calls[i] = stl
                if fn.get("name"):
                    stl["name"] = fn["name"]
                if isinstance(fn.get("arguments"), str):
                    stl["arguments"] += fn["arguments"]

    msg = {"role": "assistant", "content": "".join(content) or None}
    if tool_calls:
        msg["tool_calls"] = [
            {"id": tool_calls[i]["id"], "type": "function",
             "function": {"name": tool_calls[i]["name"],
                          "arguments": tool_calls[i]["arguments"]}}
            for i in sorted(tool_calls)
        ]
    out = {"id": "chatcmpl-" + _hex(), "object": "chat.completion",
           "created": int(time.time()), "model": model,
           "choices": [{"index": 0, "message": msg,
                        "finish_reason": finish or "stop"}],
           "usage": usage or {"prompt_tokens": 0, "completion_tokens": 0,
                              "total_tokens": 0}}
    if upstream_error is not None:
        out["_upstream_error"] = upstream_error
    return out


# ============================================================
# 六、辅助：完整 Chat JSON -> Anthropic SSE 序列；单帧错误事件
# ============================================================

def chat_json_to_anthropic_sse(chat_json, model=None):
    """完整 Chat JSON -> Anthropic SSE 帧序列（generator）。

    用于客户端要流式、但上游只给了非流式 JSON（或错误）的场景：
    message_start -> 逐 content_block start/delta/stop -> message_delta -> message_stop。
    """
    msg_obj = chat_json_to_anthropic(chat_json, model)
    base_usage = msg_obj["usage"]
    yield _ant_frame("message_start", {"message": {
        "id": msg_obj["id"], "type": "message", "role": "assistant",
        "content": [], "model": msg_obj["model"], "stop_reason": None,
        "stop_sequence": None,
        "usage": {"input_tokens": base_usage["input_tokens"], "output_tokens": 0,
                  "cache_creation_input_tokens": 0,
                  "cache_read_input_tokens": 0}}})
    for idx, block in enumerate(msg_obj["content"]):
        if block["type"] == "text":
            yield _ant_frame("content_block_start", {
                "index": idx, "content_block": {"type": "text", "text": ""}})
            yield _ant_frame("content_block_delta", {
                "index": idx, "delta": {"type": "text_delta", "text": block["text"]}})
            yield _ant_frame("content_block_stop", {"index": idx})
        elif block["type"] == "tool_use":
            yield _ant_frame("content_block_start", {
                "index": idx,
                "content_block": {"type": "tool_use", "id": block["id"],
                                   "name": block["name"], "input": {}}})
            yield _ant_frame("content_block_delta", {
                "index": idx,
                "delta": {"type": "input_json_delta",
                          "partial_json": json.dumps(block["input"],
                                                     ensure_ascii=False)}})
            yield _ant_frame("content_block_stop", {"index": idx})
    yield _ant_frame("message_delta", {
        "delta": {"stop_reason": msg_obj["stop_reason"], "stop_sequence": None},
        "usage": {"output_tokens": base_usage["output_tokens"]}})
    yield _ant_frame("message_stop", {})


def error_sse(err_type, message):
    """单个 Anthropic error 事件帧（bytes）。"""
    return _ant_frame("error", {"error": {"type": err_type, "message": message}})
