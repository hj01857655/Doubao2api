"""app/gateway.py — 数据面网关：上游客户端、入站鉴权、会话缓存、重试与 Chat 形状装配。

协议路由（app/routes/*）只做协议翻译；凡是"怎么跟豆包打交道"都在这里：
  CLIENT          上游客户端（构造失败置 None，路由以 503 拒绝）
  guard()         入站鉴权（Bearer / x-api-key，未设 key 放行）
  backoff_stream  过频退避重试（已吐内容绝不重试，避免正文重复）
  chat_sse        doubao 流 -> Chat Completions SSE 行流（协议桥的输入）
  chat_json       done 帧 -> Chat Completions JSON（协议桥非流式的输入）
"""
from __future__ import annotations

import json
import logging
import threading
import time
import uuid

from flask import jsonify, request

from app import config, doubao, usage

config.load_env()
log = logging.getLogger("doubao2api")

try:
    CLIENT = doubao.DoubaoClient()
except doubao.DoubaoError as exc:
    CLIENT = None
    log.error("启动即失败：%s", exc)


def reload_client() -> bool:
    """按当前环境变量（cookie）重建上游客户端，返回是否可用。

    扫码登录写回 .env 后走这里热替换，省掉重启；客户端不持有跨请求连接，
    重建期间的旧实例由 GC 收走。
    """
    global CLIENT
    try:
        CLIENT = doubao.DoubaoClient()
        log.info("上游客户端已按新 cookie 重建")
        return True
    except doubao.DoubaoError as exc:
        CLIENT = None
        log.error("重建上游客户端失败：%s", exc)
        return False

# conversation_id 缓存：key = OpenAI `user` / Anthropic metadata.user_id / X-Doubao-Session
_CONV_LOCK = threading.Lock()
_CONV_BY_USER: dict[str, str] = {}


# ------------------------------------------------------------------ errors
def openai_error(message: str, err_type: str = "server_error",
                 code=None, status: int = 500):
    body = {"error": {"message": message, "type": err_type,
                      "code": code, "param": None}}
    return jsonify(body), status


def err_type(exc: doubao.DoubaoError) -> str:
    """错误分类 -> OpenAI/通用 err type。不依赖 Flask 上下文，生成器里也能用。"""
    return {"auth": "authentication_error",
            "risk": "risk_control_error",
            "rate": "rate_limit_error"}.get(exc.kind, "server_error")


# ------------------------------------------------------------------ auth
def guard(anthropic: bool = False):
    """入站鉴权：设了 API_KEY/DOUBAO_API_KEY 则要求匹配（Authorization: Bearer 或
    x-api-key，后者是 Anthropic SDK 的习惯头）；未设则不校验（本地裸奔模式）。
    返回 None 表示放行，否则是带好错误形状的 401 响应。"""
    key = config.api_key()
    if not key:
        return None
    if (request.headers.get("Authorization", "") == "Bearer " + key
            or request.headers.get("x-api-key") == key):
        return None
    if anthropic:
        from app.bridges import anthropic_bridge as antb
        return jsonify(antb.error_body("authentication_error",
                                       "invalid api key")), 401
    return openai_error("invalid api key", "auth_error",
                        code="invalid_api_key", status=401)


# ------------------------------------------------------------------ messages -> text
def flatten(content) -> str:
    """OpenAI content 可能是 str 或 [{type:text,text:..}] 数组。"""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = []
        for c in content:
            if isinstance(c, dict) and c.get("type") in (None, "text"):
                parts.append(c.get("text") or "")
            elif isinstance(c, str):
                parts.append(c)
        return "\n".join(p for p in parts if p)
    return str(content or "")


def pick_text_and_history(messages) -> tuple[str, list[str]]:
    """取最后一条 user 作为本轮输入，其余作为历史。"""
    users = [m for m in messages if m.get("role") == "user"]
    if not users:
        raise ValueError("messages 里必须有至少一条 user 消息")
    newest = flatten(users[-1].get("content"))
    history = []
    for m in messages:
        if m is users[-1]:
            continue
        role = m.get("role")
        if role in ("system", "user", "assistant"):
            t = flatten(m.get("content"))
            if t:
                history.append("%s: %s" % (role, t))
    return newest, history


# ------------------------------------------------------------------ conversation cache
def conv_key(body) -> str | None:
    if body.get("user"):
        return str(body["user"])
    meta = body.get("metadata")
    if isinstance(meta, dict) and meta.get("user_id"):
        return "anth:" + str(meta["user_id"])  # Anthropic Messages 的 metadata.user_id
    return request.headers.get("X-Doubao-Session")


def resolve_conversation_id(body) -> tuple[str | None, str]:
    """返回 (要用的 conversation_id, 本次用的 key)。"""
    explicit = request.headers.get("X-Doubao-Conversation-Id") or body.get("conversation_id")
    key = conv_key(body)
    if explicit:
        return explicit, key
    if key:
        with _CONV_LOCK:
            return _CONV_BY_USER.get(key), key
    return None, key


def remember_conversation(key: str | None, conv_id: str | None):
    if not key or not conv_id:
        return
    with _CONV_LOCK:
        prev = _CONV_BY_USER.get(key)
        if prev != conv_id:
            log.info("conversation mapped key=%s -> %s", key, conv_id)
        _CONV_BY_USER[key] = conv_id


# ------------------------------------------------------------------ upstream plumbing
def backoff_stream(text, model, conv_id):
    """过频按 timeout_conf 的 retry_interval_ms 退避一次；风控/身份错误立即抛。

    关键约束：**已经往下游吐过任何内容就绝不重试**。重试会从头再跑一遍上游，
    已发出的正文会被第二次覆盖式重放，客户端拿到重复文本。
    """
    attempts = 2
    for i in range(attempts):
        emitted = False
        try:
            for item in CLIENT.stream(text, model, conv_id or ""):
                emitted = True
                yield item
            return
        except doubao.DoubaoError as exc:
            if emitted:
                log.warning("已下发内容，放弃重试（避免正文重复）kind=%s code=%s",
                            exc.kind, exc.code)
                raise
            if i + 1 < attempts and exc.retryable:
                log.warning("上游可重试错误(%s code=%s)，1s 后重试一次",
                            exc.kind, exc.code)
                time.sleep(1.0)
                continue
            raise


def collect(text, canon, conv_id):
    """非流式：跑完上游流只取 done 帧；未正常结束按 upstream 错误抛。"""
    done = None
    for kind, val in backoff_stream(text, canon, conv_id):
        if kind == "done":
            done = val
    if not done:
        raise doubao.DoubaoError("上游流未正常结束", kind="upstream")
    return done


def chat_json(done, text, canon):
    """done 帧 -> 最小 Chat Completions JSON（协议桥非流式转换器的输入）。"""
    reply = done.get("text") or ""
    return {
        "id": "chatcmpl-" + uuid.uuid4().hex[:24],
        "object": "chat.completion",
        "created": int(time.time()),
        "model": canon,
        "choices": [{"index": 0,
                     "message": {"role": "assistant", "content": reply},
                     "finish_reason": "stop"}],
        "usage": usage.approx(reply, text),
    }


def chat_sse(text, canon, conv_id, key):
    """doubao 流 -> Chat Completions SSE 行流（协议桥流式转换器的输入）。

    两个桥只认 Chat SSE 形状：role/delta 帧、带 usage 的 finish 帧、
    {"error":...} 错误帧、[DONE]。会话号在这里回写缓存。
    """
    rid = "chatcmpl-" + uuid.uuid4().hex[:24]
    created = int(time.time())

    def frame(obj):
        return "data: " + json.dumps(obj, ensure_ascii=False,
                                     separators=(",", ":")) + "\n\n"

    yield frame({"id": rid, "object": "chat.completion.chunk",
                 "created": created, "model": canon,
                 "choices": [{"index": 0, "delta": {"role": "assistant"},
                              "finish_reason": None}]})
    done = None
    try:
        for kind, val in backoff_stream(text, canon, conv_id):
            if kind == "delta":
                yield frame({"id": rid, "object": "chat.completion.chunk",
                             "created": created, "model": canon,
                             "choices": [{"index": 0, "delta": {"content": val},
                                          "finish_reason": None}]})
            elif kind == "done":
                done = val
    except doubao.DoubaoError as exc:
        log.warning("upstream error kind=%s code=%s msg=%s",
                    exc.kind, exc.code, exc)
        yield frame({"error": {"message": str(exc), "type": err_type(exc),
                               "code": exc.code}})
        yield "data: [DONE]\n\n"
        return
    if done:
        remember_conversation(key, done.get("conversation_id"))
        yield frame({"id": rid, "object": "chat.completion.chunk",
                     "created": created, "model": canon,
                     "choices": [{"index": 0, "delta": {},
                                  "finish_reason": "stop"}],
                     "usage": usage.approx(done.get("text") or "", text)})
    yield "data: [DONE]\n\n"


def sse(payload: dict) -> str:
    return "data: " + json.dumps(payload, ensure_ascii=False,
                                 separators=(",", ":")) + "\n\n"
