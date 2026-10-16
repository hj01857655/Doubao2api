# -*- coding: utf-8 -*-
"""responses_bridge.py — OpenAI Responses <-> Chat Completions 协议桥（各 2api 项目共用）

背景：grok-build / Codex 这类 agent 客户端原生只说 Responses 协议（SSE 每帧
必须带 sequence_number，类型级强制）。各 2api 上游管线只出 Chat Completions。
本模块补上中间这层：

  请求侧  responses_to_chat(body)         Responses 请求体 -> Chat 请求体
  响应侧  chat_sse_to_responses(stream)   Chat SSE 字节流 -> Responses SSE 字节流
          chat_json_to_responses(chat)    非流式 Chat JSON -> Responses JSON

字段形状以 grok-build 1.0.52 实际使用的 async-openai fork（rev 95b52eb）的
serde 必填字段为准核对过，不是猜的：
  - 每个事件必带 sequence_number（u64，本模块从 0 单调递增）
  - Response 必填：id / object / created_at / model / status / output
  - ResponseUsage 必填：input_tokens / input_tokens_details.cached_tokens /
    output_tokens / output_tokens_details.reasoning_tokens / total_tokens
  - ErrorObject 必填：code + message；IncompleteDetails 必填：reason

各项目的鉴权/签名不碰，全部留在各自 server.py 的既有管线里。
"""

import json
import re
import time
import uuid


def _hex(n=24):
    return uuid.uuid4().hex[:n]


# ============================================================
# 一、请求侧：Responses 请求体 -> Chat Completions 请求体
# ============================================================

def _content_part_to_chat(part):
    """Responses 内容块 -> Chat 内容块（不支持的返回 None 丢弃）。"""
    if not isinstance(part, dict):
        return None
    t = part.get("type")
    if t in ("input_text", "output_text", "text"):
        return {"type": "text", "text": part.get("text") or ""}
    if t == "input_image":
        url = part.get("image_url")
        if isinstance(url, dict):
            url = url.get("url")
        if isinstance(url, str) and url:
            return {"type": "image_url", "image_url": {"url": url}}
    # input_file / input_image(file_id) 等 Chat 表达不了的块：丢弃
    return None


def _to_chat_content(content):
    """Responses message.content（字符串或块数组）-> Chat content。"""
    if content is None:
        return ""
    if isinstance(content, str):
        return content
    if not isinstance(content, list):
        return str(content)
    parts = []
    for p in content:
        if isinstance(p, str):
            parts.append({"type": "text", "text": p})
            continue
        c = _content_part_to_chat(p)
        if c is not None:
            parts.append(c)
    if not parts:
        return ""
    if len(parts) == 1 and parts[0]["type"] == "text":
        return parts[0]["text"]
    return parts


def _input_to_messages(body):
    """instructions + input -> Chat messages。

    - instructions -> 首条 system
    - input 为字符串 -> 单条 user
    - 连续 function_call 合并为一条 assistant.tool_calls 消息
    - function_call_output -> role=tool
    - reasoning / item_reference 等无 Chat 对应的项：跳过
    """
    messages = []
    instructions = body.get("instructions")
    if isinstance(instructions, str) and instructions:
        messages.append({"role": "system", "content": instructions})

    inp = body.get("input")
    if inp is None:
        return messages
    if isinstance(inp, str):
        messages.append({"role": "user", "content": inp})
        return messages
    if not isinstance(inp, list):
        return messages

    pending_calls = []

    def flush_calls():
        if not pending_calls:
            return
        messages.append({
            "role": "assistant",
            "content": None,
            "tool_calls": [
                {
                    "id": c.get("call_id") or "call_" + _hex(),
                    "type": "function",
                    "function": {
                        "name": c.get("name") or "",
                        "arguments": c.get("arguments") or "",
                    },
                }
                for c in pending_calls
            ],
        })
        pending_calls.clear()

    for item in inp:
        if isinstance(item, str):
            flush_calls()
            messages.append({"role": "user", "content": item})
            continue
        if not isinstance(item, dict):
            continue
        t = item.get("type")
        if t is None and ("role" in item or "content" in item):
            # EasyInput 简写：{"role","content"} 不带 type（OpenAI SDK 与
            # deepseek-harness 等直发此形状），按 message 处理
            t = "message"
        if t == "message":
            flush_calls()
            role = item.get("role") or "user"
            if role == "developer":
                role = "system"
            if role not in ("system", "user", "assistant"):
                role = "user"
            messages.append({"role": role, "content": _to_chat_content(item.get("content"))})
        elif t == "function_call":
            pending_calls.append(item)
        elif t == "function_call_output":
            flush_calls()
            out = item.get("output")
            if not isinstance(out, str):
                out = json.dumps(out, ensure_ascii=False) if out is not None else ""
            messages.append({
                "role": "tool",
                "tool_call_id": item.get("call_id") or "",
                "content": out,
            })
        # 其它类型（reasoning / item_reference / ...）：无 Chat 对应，跳过
    flush_calls()
    return messages


def _tools_to_chat(tools):
    """Responses tools -> Chat tools；内置工具（web_search 等）Chat 表达不了，丢弃。"""
    if not isinstance(tools, list):
        return None
    out = []
    for t in tools:
        if not isinstance(t, dict) or t.get("type") != "function":
            continue
        out.append({
            "type": "function",
            "function": {
                "name": t.get("name") or "",
                "description": t.get("description") or "",
                "parameters": t.get("parameters") or {"type": "object", "properties": {}},
            },
        })
    return out or None


def _tool_choice_to_chat(tc):
    if tc is None:
        return None
    if isinstance(tc, str):
        return tc if tc in ("auto", "required", "none") else "auto"
    if isinstance(tc, dict) and tc.get("type") == "function":
        return {"type": "function", "function": {"name": tc.get("name") or ""}}
    return None


def responses_to_chat(body):
    """Responses API 请求体 -> Chat Completions 请求体。

    只搬 Chat 能表达的字段；reasoning / text / store / previous_response_id
    等无对应项直接丢弃（v1）。返回的 dict 可直接交给既有 Chat 上游管线。
    """
    chat = {
        "model": body.get("model") or "",
        "messages": _input_to_messages(body),
        "stream": bool(body.get("stream", False)),
    }
    tools = _tools_to_chat(body.get("tools"))
    if tools:
        chat["tools"] = tools
    tc = _tool_choice_to_chat(body.get("tool_choice"))
    if tc is not None:
        chat["tool_choice"] = tc
    for src, dst in (
        ("temperature", "temperature"),
        ("top_p", "top_p"),
        ("max_output_tokens", "max_tokens"),
        ("parallel_tool_calls", "parallel_tool_calls"),
        ("stream_options", "stream_options"),
    ):
        if body.get(src) is not None:
            chat[dst] = body[src]
    return chat


# ============================================================
# 二、响应侧公共：输出 item 构造 + usage 映射
# ============================================================

def _reasoning_item(item_id, text, status="in_progress"):
    # summary 是必填字段；v1 给空数组（流式思考内容已经通过
    # reasoning_summary_text.delta/done 帧交付给客户端，这里只保 transcript 骨架）
    return {
        "type": "reasoning",
        "id": item_id,
        "summary": [],
        "status": status,
    }


def _message_item(item_id, text, status="in_progress"):
    return {
        "type": "message",
        "id": item_id,
        "role": "assistant",
        "status": status,
        "content": [
            {"type": "output_text", "text": text, "annotations": []},
        ],
    }


def _function_item(item_id, call_id, name, arguments, status="in_progress"):
    return {
        "type": "function_call",
        "id": item_id,
        "call_id": call_id,
        "name": name,
        "arguments": arguments or "",
        "status": status,
    }


def _usage_from_chat(u):
    """Chat usage -> Responses usage（五个字段全部必填，一个都不能省）。"""
    if not isinstance(u, dict):
        return None
    pt = int(u.get("prompt_tokens") or 0)
    ct = int(u.get("completion_tokens") or 0)
    rt = 0
    det = u.get("completion_tokens_details")
    if isinstance(det, dict):
        rt = int(det.get("reasoning_tokens") or 0)
    cached = 0
    pdet = u.get("prompt_tokens_details")
    if isinstance(pdet, dict):
        cached = int(pdet.get("cached_tokens") or 0)
    total = int(u.get("total_tokens") or (pt + ct))
    return {
        "input_tokens": pt,
        "input_tokens_details": {"cached_tokens": cached},
        "output_tokens": ct,
        "output_tokens_details": {"reasoning_tokens": rt},
        "total_tokens": total,
    }


# ============================================================
# 三、流式：Chat SSE 字节流 -> Responses SSE 字节流
# ============================================================

class _StreamState:
    """单条流的簿记：序号计数器、输出 item、收尾信息。"""

    def __init__(self, model):
        self.model = model or ""
        self.resp_id = "resp_" + _hex(32)
        self.created_at = int(time.time())
        self.seq = 0
        self._next_idx = 0
        self.usage = None
        self.finish_reason = None
        self.upstream_error = None   # (code, message)
        self.interrupted = None      # 上游断流异常
        self.items = {}              # output_index -> item dict
        self.order = []              # output_index 插入序
        self.reasoning = None        # {item_id, output_index, buf}
        self.text = None             # {item_id, output_index, buf}
        self.tools = {}              # chat tool_calls index -> {...}

    def register(self, item):
        idx = self._next_idx
        self._next_idx += 1
        self.items[idx] = item
        self.order.append(idx)
        return idx

    def response_obj(self, status, usage=None, error=None, incomplete_details=None):
        """组装 Response 对象（六个必填字段一个不少）。"""
        obj = {
            "id": self.resp_id,
            "object": "response",
            "created_at": self.created_at,
            "model": self.model,
            "status": status,
            "output": [self.items[i] for i in self.order],
        }
        if usage is not None:
            obj["usage"] = usage
        if error is not None:
            obj["error"] = error
        if incomplete_details is not None:
            obj["incomplete_details"] = incomplete_details
        if status == "completed":
            obj["completed_at"] = int(time.time())
        return obj


def chat_sse_to_responses(stream, model):
    """Chat SSE 原始行流 -> Responses SSE 帧流（generator）。

    stream: 迭代器，每项是原始行（bytes 或 str，带 "data: " 前缀）；
            可直接传 upstream.iter_lines(decode_unicode=False)，
            也可传项目自己的 data 帧生成器。
    产出: 每项一个完整帧 b"data: {json}\\n\\n"，每帧必带 sequence_number。
    收尾: 保证以 response.completed / response.incomplete / response.failed
          之一结束；不发 [DONE]（那是 Chat 惯例，Responses 没有）。
    """
    st = _StreamState(model)

    def frame(event):
        event["sequence_number"] = st.seq
        st.seq += 1
        return (b"data: " + json.dumps(event, ensure_ascii=False,
                                       separators=(",", ":")).encode("utf-8")
                + b"\n\n")

    def ensure_reasoning():
        """首次思考内容：登记 item 并返回 (item_id, output_index, added 帧或 None)。"""
        if st.reasoning is None:
            item_id = "rs_" + _hex()
            idx = st.register(_reasoning_item(item_id, ""))
            st.reasoning = {"item_id": item_id, "output_index": idx, "buf": ""}
            return item_id, idx, frame({"type": "response.output_item.added",
                                        "output_index": idx,
                                        "item": st.items[idx]})
        r = st.reasoning
        return r["item_id"], r["output_index"], None

    def ensure_text():
        if st.text is None:
            item_id = "msg_" + _hex()
            idx = st.register(_message_item(item_id, ""))
            st.text = {"item_id": item_id, "output_index": idx, "buf": ""}
            return item_id, idx, frame({"type": "response.output_item.added",
                                        "output_index": idx,
                                        "item": st.items[idx]})
        t = st.text
        return t["item_id"], t["output_index"], None

    yield frame({"type": "response.created",
                 "response": st.response_obj("in_progress")})
    yield frame({"type": "response.in_progress",
                 "response": st.response_obj("in_progress")})

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

            # 项目自定义错误帧：{"error":{"message":...}}（CodeArts 上游错误帧
            # 转出来的 OpenAI error 帧）或 {"error_code","error_msg"} 原样帧
            err = chunk.get("error")
            if isinstance(err, dict) and err.get("message"):
                st.upstream_error = (str(err.get("code") or "upstream_error"),
                                     str(err["message"]))
                break
            if chunk.get("error_code") or chunk.get("error_msg"):
                st.upstream_error = (str(chunk.get("error_code") or "upstream_error"),
                                     str(chunk.get("error_msg") or "upstream error"))
                break

            if isinstance(chunk.get("usage"), dict):
                st.usage = _usage_from_chat(chunk["usage"])

            choices = chunk.get("choices")
            if not (isinstance(choices, list) and choices
                    and isinstance(choices[0], dict)):
                continue
            choice = choices[0]
            fr = choice.get("finish_reason")
            if fr:
                st.finish_reason = fr
            delta = choice.get("delta")
            if not isinstance(delta, dict):
                continue

            # 1) reasoning_content -> 思考摘要文本 delta
            rc = delta.get("reasoning_content")
            if isinstance(rc, str) and rc:
                item_id, idx, ev = ensure_reasoning()
                if ev is not None:
                    yield ev
                st.reasoning["buf"] += rc
                yield frame({"type": "response.reasoning_summary_text.delta",
                             "item_id": item_id, "output_index": idx,
                             "summary_index": 0, "delta": rc})

            # 2) content -> 正文文本 delta
            ct = delta.get("content")
            if isinstance(ct, str) and ct:
                item_id, idx, ev = ensure_text()
                if ev is not None:
                    yield ev
                st.text["buf"] += ct
                yield frame({"type": "response.output_text.delta",
                             "item_id": item_id, "output_index": idx,
                             "content_index": 0, "delta": ct})

            # 3) tool_calls -> function_call item + 参数 delta
            tcs = delta.get("tool_calls")
            if isinstance(tcs, list):
                for tc in tcs:
                    if not isinstance(tc, dict):
                        continue
                    i = tc.get("index")
                    if not isinstance(i, int):
                        i = len(st.tools)
                    fn = tc.get("function")
                    if not isinstance(fn, dict):
                        fn = {}
                    stl = st.tools.get(i)
                    if stl is None:
                        call_id = tc.get("id") or "call_" + _hex()
                        item_id = "fc_" + _hex()
                        idx = st.register(_function_item(item_id, call_id,
                                                         fn.get("name") or "", ""))
                        stl = {"item_id": item_id, "call_id": call_id,
                               "name": fn.get("name") or "", "args": "",
                               "output_index": idx}
                        st.tools[i] = stl
                        yield frame({"type": "response.output_item.added",
                                     "output_index": idx,
                                     "item": st.items[idx]})
                    name = fn.get("name")
                    if isinstance(name, str) and name:
                        stl["name"] = name
                        st.items[stl["output_index"]]["name"] = name
                    args = fn.get("arguments")
                    if isinstance(args, str) and args:
                        stl["args"] += args
                        st.items[stl["output_index"]]["arguments"] = stl["args"]
                        yield frame({"type": "response.function_call_arguments.delta",
                                     "item_id": stl["item_id"],
                                     "output_index": stl["output_index"],
                                     "delta": args})
    except Exception as e:
        # 上游断流（读超时/连接重置等）：不让异常掀掉 SSE，收尾帧兜底
        st.interrupted = e

    # 逐项收尾：先发各类型 done 帧（按 output 序），再发 output_item.done
    for idx in st.order:
        item = st.items[idx]
        if item["type"] == "reasoning":
            r = st.reasoning
            if r is not None and r["output_index"] == idx and r["buf"]:
                yield frame({"type": "response.reasoning_summary_text.done",
                             "item_id": item["id"], "output_index": idx,
                             "summary_index": 0, "text": r["buf"]})
            item["status"] = "completed"
        elif item["type"] == "message":
            t = st.text
            if t is not None and t["output_index"] == idx:
                yield frame({"type": "response.output_text.done",
                             "item_id": item["id"], "output_index": idx,
                             "content_index": 0, "text": t["buf"]})
                # 终帧必须携带全文：把攒的 buffer 写回 item，
                # 否则 output_item.done / response.completed 里正文为空
                item["content"][0]["text"] = t["buf"]
            item["status"] = "completed"
        elif item["type"] == "function_call":
            stl = next((s for s in st.tools.values()
                        if s["output_index"] == idx), None)
            name = stl["name"] if stl else (item.get("name") or "")
            args = stl["args"] if stl else (item.get("arguments") or "")
            yield frame({"type": "response.function_call_arguments.done",
                         "name": name, "item_id": item["id"],
                         "output_index": idx, "arguments": args})
            item["status"] = "completed"
        yield frame({"type": "response.output_item.done",
                     "output_index": idx, "item": item})

    # 终局事件：failed / incomplete / completed 三选一
    if st.upstream_error is not None:
        code, msg = st.upstream_error
        yield frame({"type": "response.failed",
                     "response": st.response_obj(
                         "failed", usage=st.usage,
                         error={"code": code, "message": msg})})
    elif st.interrupted is not None:
        if st.order:
            yield frame({"type": "response.incomplete",
                         "response": st.response_obj(
                             "incomplete", usage=st.usage,
                             incomplete_details={
                                 "reason": "stream_interrupted"})})
        else:
            yield frame({"type": "response.failed",
                         "response": st.response_obj(
                             "failed", usage=st.usage,
                             error={"code": "stream_interrupted",
                                    "message": str(st.interrupted)})})
    elif st.finish_reason == "length":
        yield frame({"type": "response.incomplete",
                     "response": st.response_obj(
                         "incomplete", usage=st.usage,
                         incomplete_details={
                             "reason": "max_output_tokens"})})
    else:
        yield frame({"type": "response.completed",
                     "response": st.response_obj("completed", usage=st.usage)})


# ============================================================
# 四、非流式：Chat JSON -> Responses JSON
# ============================================================

def chat_json_to_responses(chat, model=None):
    """非流式 Chat Completions JSON -> Responses 完整 Response 对象（dict）。

    用法：项目管线把上游（无论流不流）聚合成单个 Chat JSON 后调用本函数，
    返回的 dict 直接 jsonify 给客户端即可。
    """
    st = _StreamState(model or chat.get("model") or "")
    msg = {}
    choices = chat.get("choices")
    if isinstance(choices, list) and choices and isinstance(choices[0], dict):
        m = choices[0].get("message")
        if isinstance(m, dict):
            msg = m
        fr = choices[0].get("finish_reason")
        if fr:
            st.finish_reason = fr
    if isinstance(chat.get("usage"), dict):
        st.usage = _usage_from_chat(chat["usage"])

    # output 顺序：reasoning -> message -> function_call（与流式一致）
    rc = msg.get("reasoning_content")
    if isinstance(rc, str) and rc:
        st.register(_reasoning_item("rs_" + _hex(), rc, "completed"))
    text = msg.get("content")
    if isinstance(text, str) and text:
        st.register(_message_item("msg_" + _hex(), text, "completed"))
    tcs = msg.get("tool_calls")
    if isinstance(tcs, list):
        for tc in tcs:
            if not isinstance(tc, dict):
                continue
            fn = tc.get("function")
            if not isinstance(fn, dict):
                fn = {}
            st.register(_function_item("fc_" + _hex(),
                                       tc.get("id") or "call_" + _hex(),
                                       fn.get("name") or "",
                                       fn.get("arguments") or "", "completed"))

    if st.finish_reason == "length":
        return st.response_obj("incomplete", usage=st.usage,
                               incomplete_details={
                                   "reason": "max_output_tokens"})
    return st.response_obj("completed", usage=st.usage)


# ============================================================
# 五、辅助：错误 Response 对象 + 非流式结果 -> 最小 SSE 序列
# ============================================================

def error_response_obj(model, code, message):
    """构造最小可用的 failed Response 对象（六个必填字段齐全）。"""
    return {
        "id": "resp_" + _hex(32),
        "object": "response",
        "created_at": int(time.time()),
        "model": model or "",
        "status": "failed",
        "output": [],
        "error": {"code": code or "upstream_error",
                  "message": message or "upstream error"},
    }


def response_json_to_sse(response_obj):
    """完整 Responses JSON -> 最小 SSE 帧序列（generator）。

    用于客户端要流式、但上游只给了非流式结果（或错误）的场景：
    created -> in_progress -> 逐 item added -> 终局事件（按 status 选）。
    每帧带 sequence_number，与流式路径的帧形状完全一致。
    """
    seq = 0

    def frame(event):
        nonlocal seq
        event["sequence_number"] = seq
        seq += 1
        return (b"data: " + json.dumps(event, ensure_ascii=False,
                                       separators=(",", ":")).encode("utf-8")
                + b"\n\n")

    base = dict(response_obj)
    base["status"] = "in_progress"
    base["output"] = []
    base.pop("completed_at", None)
    base.pop("error", None)
    base.pop("incomplete_details", None)
    yield frame({"type": "response.created", "response": base})
    yield frame({"type": "response.in_progress", "response": base})
    for idx, item in enumerate(response_obj.get("output") or []):
        yield frame({"type": "response.output_item.added",
                     "output_index": idx, "item": item})
    status = response_obj.get("status") or "completed"
    terminal = {"completed": "response.completed",
                "failed": "response.failed",
                "incomplete": "response.incomplete"}.get(
                status, "response.completed")
    yield frame({"type": terminal, "response": response_obj})


# ============================================================
# 六、antml 文本工具调用回收（Qwen 系不稳定工具调用形态）
# ============================================================
#
# Qwen 系模型在部分系统提示下会把工具调用写成 antml 纯文本而不是结构化
# tool_calls：
#     <invoke name="Bash">
#     <parameter name="command">…
#     <parameter name="timeout">600000
#     </invoke>
# 客户端（claude-cli / grok / deepseek-harness）只认结构化 tool_calls，文本透传
# 会直接卡死回合（finish=stop、无工具调用）。InvokeSalvage 把文本拆回
# tool_call 事件，由调用方（各 server.py）转成各自路由的协议帧。
# 回收器无副作用：解析失败一律降级为纯文本透传，绝不丢内容。

_INVOKE_START = '<invoke name="'
_INVOKE_END = '</invoke>'
_PARAM_RE = re.compile(r'<(\w+) name="([^"]*)">(.*?)</\1>', re.S)


def _coerce_param(v):
    """XML 参数值是裸文本：数字/布尔还原成 JSON 类型（如 timeout 600000），其余保持字符串。"""
    try:
        x = json.loads(v)
        if isinstance(x, (int, float, bool)):
            return x
    except (json.JSONDecodeError, ValueError):
        pass
    return v


class InvokeSalvage:
    """正文文本工具调用的流式回收器（有界前瞻）。

    feed(text) -> [(kind, payload), ...]
        ("text", str)        —— 透传正文
        ("tool_call", dict)  —— {"name": …, "params": {k: v}}
    flush() -> 同形状，流末清空残留缓冲（未闭合块退回纯文本）。

    只扣住"可能是 invoke 起始标记"的缓冲尾部（≤14 字符），其余正文实时放行；
    解析失败一律降级为纯文本透传，绝不丢内容。
    """

    def __init__(self):
        self.buf = ""

    def feed(self, text):
        if not text:
            return []
        out = []
        self.buf += text
        while True:
            start = self.buf.find(_INVOKE_START)
            if start == -1:
                hold = self._partial_tail()
                cut = len(self.buf) - hold
                head, self.buf = self.buf[:cut], self.buf[cut:]
                if head:
                    out.append(("text", head))
                return out
            end = self.buf.find(_INVOKE_END, start)
            if end == -1:
                # 块未闭合：放行标记前文本，从标记起扣住
                if start:
                    out.append(("text", self.buf[:start]))
                self.buf = self.buf[start:]
                return out
            # 完整块：前段放行、解析块、继续扫剩余
            if start:
                out.append(("text", self.buf[:start]))
            block = self.buf[start:end + len(_INVOKE_END)]
            tc = self._parse(block)
            if tc is None:
                out.append(("text", block))  # 解析失败：整块降级纯文本
            else:
                out.append(("tool_call", tc))
            self.buf = self.buf[end + len(_INVOKE_END):]

    def flush(self):
        if self.buf:
            out, self.buf = [("text", self.buf)], ""
            return out
        return []

    def _partial_tail(self):
        """缓冲尾部是起始标记部分前缀的字符数（0 = 无）。"""
        m = min(len(self.buf), len(_INVOKE_START))
        for n in range(m, 0, -1):
            if self.buf[-n:] == _INVOKE_START[:n]:
                return n
        return 0

    @staticmethod
    def _parse(block):
        m = re.match(r'<invoke name="([^"]+)">\s*', block)
        if not m:
            return None
        inner = block[m.end():-len(_INVOKE_END)]
        params = {}
        for pm in _PARAM_RE.finditer(inner):
            if pm.group(1) != "parameter":
                continue
            params[pm.group(2)] = _coerce_param(pm.group(3))
        return {"name": m.group(1), "params": params}
