"""app/routes/openai_responses.py — OpenAI Responses 路由（/v1/responses）。"""
from __future__ import annotations

import logging

from flask import Blueprint, Response, jsonify, request

from app import doubao, gateway
from app.bridges import responses_bridge as respb

bp = Blueprint("openai_responses", __name__)
log = logging.getLogger("doubao2api")


@bp.post("/v1/responses")
def openai_responses():
    denied = gateway.guard()
    if denied:
        return denied
    if gateway.CLIENT is None:
        return jsonify(respb.error_response_obj(
            None, "auth_error", "服务端未配置 DOUBAO_COOKIE")), 503
    body = request.get_json(silent=True) or {}
    want_stream = bool(body.get("stream"))
    model_name = body.get("model") or doubao.DEFAULT_MODEL
    try:
        canon, _mode = doubao.resolve_model(model_name)
    except doubao.DoubaoError as exc:
        return jsonify(respb.error_response_obj(model_name,
                                                "invalid_request_error",
                                                str(exc))), 400

    try:
        chat_req = respb.responses_to_chat(body)
        if not chat_req["messages"]:
            return jsonify(respb.error_response_obj(model_name,
                                                    "invalid_request_error",
                                                    "input 不能为空")), 400
        newest, history = gateway.pick_text_and_history(chat_req["messages"])
    except ValueError as exc:
        return jsonify(respb.error_response_obj(model_name,
                                                "invalid_request_error",
                                                str(exc))), 400

    conv_id, key = gateway.resolve_conversation_id(body)
    if not conv_id and history:
        newest = "\n".join(history) + "\n\nuser: " + newest

    def fail_from(exc: doubao.DoubaoError):
        log.warning("upstream error kind=%s code=%s msg=%s",
                    exc.kind, exc.code, exc)
        return jsonify(respb.error_response_obj(canon, exc.kind, str(exc))), \
            exc.to_http()

    if not want_stream:
        try:
            done = gateway.collect(newest, canon, conv_id)
        except doubao.DoubaoError as exc:
            return fail_from(exc)
        gateway.remember_conversation(key, done.get("conversation_id"))
        out = respb.chat_json_to_responses(gateway.chat_json(done, newest, canon),
                                           canon)
        resp = jsonify(out)
        if done.get("conversation_id"):
            resp.headers["X-Doubao-Conversation-Id"] = done["conversation_id"]
        return resp

    def gen():
        yield from respb.chat_sse_to_responses(
            gateway.chat_sse(newest, canon, conv_id, key), canon)

    return Response(gen(), headers={"Content-Type": "text/event-stream",
                                    "Cache-Control": "no-cache",
                                    "X-Accel-Buffering": "no"})
