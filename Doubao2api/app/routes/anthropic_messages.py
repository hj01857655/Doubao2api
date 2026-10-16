"""app/routes/anthropic_messages.py — Anthropic Messages 路由（/v1/messages）。"""
from __future__ import annotations

import logging

from flask import Blueprint, Response, jsonify, request

from app import doubao, gateway
from app.bridges import anthropic_bridge as antb

bp = Blueprint("anthropic_messages", __name__)
log = logging.getLogger("doubao2api")


@bp.post("/v1/messages")
def anthropic_messages():
    denied = gateway.guard(anthropic=True)
    if denied:
        return denied
    if gateway.CLIENT is None:
        return jsonify(antb.error_body("authentication_error",
                                       "服务端未配置 DOUBAO_COOKIE")), 503
    body = request.get_json(silent=True) or {}
    want_stream = bool(body.get("stream"))
    model_name = body.get("model") or doubao.DEFAULT_MODEL
    try:
        canon, _mode = doubao.resolve_model(model_name)
    except doubao.DoubaoError as exc:
        return jsonify(antb.error_body("invalid_request_error", str(exc))), 400

    try:
        chat_req = antb.anthropic_to_chat(body)
        if not chat_req["messages"]:
            return jsonify(antb.error_body("invalid_request_error",
                                           "messages 不能为空")), 400
        newest, history = gateway.pick_text_and_history(chat_req["messages"])
    except ValueError as exc:
        return jsonify(antb.error_body("invalid_request_error", str(exc))), 400

    conv_id, key = gateway.resolve_conversation_id(body)
    if not conv_id and history:
        newest = "\n".join(history) + "\n\nuser: " + newest

    def fail_from(exc: doubao.DoubaoError):
        log.warning("upstream error kind=%s code=%s msg=%s",
                    exc.kind, exc.code, exc)
        return jsonify(antb.error_body(antb.err_type_for_http(exc.to_http()),
                                       str(exc))), exc.to_http()

    if not want_stream:
        try:
            done = gateway.collect(newest, canon, conv_id)
        except doubao.DoubaoError as exc:
            return fail_from(exc)
        gateway.remember_conversation(key, done.get("conversation_id"))
        out = antb.chat_json_to_anthropic(gateway.chat_json(done, newest, canon),
                                          canon)
        out["doubao"] = {"conversation_id": done.get("conversation_id"),
                         "question_id": done.get("question_id"),
                         "answer_message_id": done.get("answer_message_id")}
        resp = jsonify(out)
        if done.get("conversation_id"):
            resp.headers["X-Doubao-Conversation-Id"] = done["conversation_id"]
        return resp

    def gen():
        yield from antb.chat_sse_to_anthropic(
            gateway.chat_sse(newest, canon, conv_id, key), canon)

    return Response(gen(), headers={"Content-Type": "text/event-stream",
                                    "Cache-Control": "no-cache",
                                    "X-Accel-Buffering": "no"})
