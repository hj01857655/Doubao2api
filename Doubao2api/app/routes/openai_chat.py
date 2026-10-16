"""app/routes/openai_chat.py — OpenAI Chat Completions 路由（/v1/chat/completions）。"""
from __future__ import annotations

import logging
import time
import uuid

from flask import Blueprint, Response, jsonify, request

from app import doubao, gateway, usage

bp = Blueprint("openai_chat", __name__)
log = logging.getLogger("doubao2api")


@bp.post("/v1/chat/completions")
def chat_completions():
    denied = gateway.guard()
    if denied:
        return denied
    if gateway.CLIENT is None:
        return gateway.openai_error("服务端未配置 DOUBAO_COOKIE", "auth_error",
                                    status=503)
    body = request.get_json(silent=True) or {}
    messages = body.get("messages") or []
    if not isinstance(messages, list) or not messages:
        return gateway.openai_error("messages 不能为空", "invalid_request_error",
                                    status=400)

    want_stream = bool(body.get("stream"))
    model_name = body.get("model") or doubao.DEFAULT_MODEL
    try:
        canon, _mode = doubao.resolve_model(model_name)
    except doubao.DoubaoError as exc:
        return gateway.openai_error(str(exc), "invalid_request_error", status=400)

    try:
        text, history = gateway.pick_text_and_history(messages)
    except ValueError as exc:
        return gateway.openai_error(str(exc), "invalid_request_error", status=400)

    conv_id, key = gateway.resolve_conversation_id(body)
    # 新会话要把历史带进去，否则上下文丢失
    if not conv_id and history:
        text = "\n".join(history) + "\n\nuser: " + text

    rid = "chatcmpl-" + uuid.uuid4().hex[:24]
    created = int(time.time())

    def fail_from(exc: doubao.DoubaoError):
        log.warning("upstream error kind=%s code=%s msg=%s",
                    exc.kind, exc.code, exc)
        return gateway.openai_error(str(exc), gateway.err_type(exc),
                                    exc.code, exc.to_http())

    if not want_stream:
        try:
            done = None
            for kind, val in gateway.backoff_stream(text, canon, conv_id):
                if kind == "done":
                    done = val
        except doubao.DoubaoError as exc:
            return fail_from(exc)
        if not done:
            return gateway.openai_error("上游流未正常结束", "server_error", status=502)
        gateway.remember_conversation(key, done.get("conversation_id"))
        reply = done["text"]
        return jsonify({
            "id": rid, "object": "chat.completion", "created": created,
            "model": canon,
            "choices": [{"index": 0,
                         "message": {"role": "assistant", "content": reply},
                         "finish_reason": "stop"}],
            "usage": usage.approx(reply, text),
            "doubao": {"conversation_id": done.get("conversation_id"),
                       "question_id": done.get("question_id"),
                       "answer_message_id": done.get("answer_message_id"),
                       "model_type": done.get("model_type"),
                       "suggestions": done.get("suggestions"),
                       "log_id": done.get("log_id")},
        })

    def gen():
        # 首帧：role
        yield gateway.sse({"id": rid, "object": "chat.completion.chunk",
                           "created": created, "model": canon,
                           "choices": [{"index": 0, "delta": {"role": "assistant"},
                                        "finish_reason": None}]})
        done = None
        finished_early = False
        try:
            for kind, val in gateway.backoff_stream(text, canon, conv_id):
                if kind == "delta":
                    yield gateway.sse({"id": rid, "object": "chat.completion.chunk",
                                       "created": created, "model": canon,
                                       "choices": [{"index": 0,
                                                    "delta": {"content": val},
                                                    "finish_reason": None}]})
                elif kind == "body_end":
                    # 正文已完整，立刻给 finish_reason（实测比 end_type:3 早 2~3 秒）
                    finished_early = True
                    yield gateway.sse({"id": rid, "object": "chat.completion.chunk",
                                       "created": created, "model": canon,
                                       "choices": [{"index": 0, "delta": {},
                                                    "finish_reason": "stop"}],
                                       "doubao": {"conversation_id": val.get("conversation_id"),
                                                  "model_type": val.get("model_type")}})
                elif kind == "done":
                    done = val
        except doubao.DoubaoError as exc:
            # 流已开始，HTTP 状态改不了；只能以 SSE error 帧收尾。
            # 注意：这里绝不能用 jsonify/fail_from —— 生成器执行时
            # Flask 的请求上下文已经弹出，会抛 Working outside of application context。
            log.warning("upstream error kind=%s code=%s msg=%s",
                        exc.kind, exc.code, exc)
            yield gateway.sse({"error": {"message": str(exc),
                                         "type": gateway.err_type(exc),
                                         "code": exc.code,
                                         "http_status": exc.to_http()}})
            yield "data: [DONE]\n\n"
            return
        if done:
            gateway.remember_conversation(key, done.get("conversation_id"))
            if not finished_early:      # 兜底：上游没发 end_type:1 也要给 finish_reason
                yield gateway.sse({"id": rid, "object": "chat.completion.chunk",
                                   "created": created, "model": canon,
                                   "choices": [{"index": 0, "delta": {},
                                                "finish_reason": "stop"}]})
            yield gateway.sse({"id": rid, "object": "chat.completion.chunk",
                               "created": created, "model": canon,
                               "choices": [],
                               "doubao": {"conversation_id": done.get("conversation_id"),
                                          "question_id": done.get("question_id"),
                                          "answer_message_id": done.get("answer_message_id"),
                                          "model_type": done.get("model_type"),
                                          "suggestions": done.get("suggestions"),
                                          "log_id": done.get("log_id")}})
        yield "data: [DONE]\n\n"

    headers = {"Content-Type": "text/event-stream", "Cache-Control": "no-cache",
               "X-Accel-Buffering": "no"}
    if conv_id:
        headers["X-Doubao-Conversation-Id"] = conv_id
    return Response(gen(), headers=headers)
