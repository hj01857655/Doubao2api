"""doubao.py — 豆包 /chat/completion 上游客户端。

契约来源：docs/03（请求面）、docs/04 + docs/05（响应面与探测修正）。
设计要点（都是实测结论，勿凭感觉改）：

1. **不实现任何签名**。`a_bogus`/`msToken` 在本端点不被校验（docs/05 §1.1）。
2. **设备 query 必须成套自洽**，砍参数会触发风控滑块 `710022004`（docs/05 §1.3）。
3. **鉴权只需 10 个 cookie 名**（docs/05 §1.2）。
4. 正文片段可能来自 `CHUNK_DELTA` 或 `STREAM_CHUNK(patch_object:1)`，二者**不重叠**，
   拼接即权威全文（docs/05 §2）。`end_type:1` 的 `msg_finish_attr.brief` 只是**会话列表预览串**：
   实测会吃掉换行（`1\n2\n…12` → `123456789101112`），长回答还可能截断 → 绝不用于覆盖正文。
5. `end_type:1` = 正文已完整（此时即可发 finish_reason）；`end_type:3` = 流真正结束
   （其后才是建议 `sp_v2` 帧）。实测两者相差 2~3 秒，故解析器在 `end_type:1` 额外发
   `body_end` 事件。
6. 错误有三支：HTTP 非 200、`event: gateway-error`、`event: STREAM_ERROR`。

零第三方依赖（stdlib `http.client`）。
"""
from __future__ import annotations

import http.client
import json
import logging
import os
import ssl
import threading
import time
import uuid
from dataclasses import dataclass, field
from urllib.parse import quote

log = logging.getLogger("doubao")

HOST = "www.doubao.com"
COMPLETION_PATH = "/chat/completion"

# ---------------------------------------------------------------- 设备面常量
# 与抓包同一套自洽值。fp = verify_<device_id>、tea_uuid = device_id 是强约束，
# 三者不一致会被风控判成异常设备（docs/05 §1.3）。可用环境变量覆盖以便多账号轮换。
DEVICE_ID = os.environ.get("DOUBAO_DEVICE_ID", "854749611058841")
WEB_ID = os.environ.get("DOUBAO_WEB_ID", "7696568307852379690")
PC_VERSION = os.environ.get("DOUBAO_PC_VERSION", "2.32.5")
CHROMIUM_VERSION = os.environ.get("DOUBAO_CHROMIUM_VERSION", "147.0.7727.149")
USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/%s Safari/537.36 SamanthaDoubao/%s"
    % (CHROMIUM_VERSION.split(".")[0], PC_VERSION)
)

BASE_QUERY = {
    "aid": "582478",
    "real_aid": "582478",
    "channel": "win",
    "chromium_version": CHROMIUM_VERSION,
    "client_platform": "pc_client",
    "device_id": DEVICE_ID,
    "device_platform": "web",
    "doubao_device_platform": "desktop",
    "doubao_pc_version": PC_VERSION,
    "fp": "verify_" + DEVICE_ID,
    "language": "zh",
    "pc_version": PC_VERSION,
    "pkg_type": "release_version",
    "region": "CN",
    "runtime": "web",
    "runtime_version": "3.41.2",
    "samantha_web": "1",
    "sys_region": "CN",
    "tea_uuid": DEVICE_ID,
    "tz_name": "Asia/Shanghai",
    "use-olympus-account": "1",
    "version_code": "20800",
    "web_id": WEB_ID,
    "web_platform": "desktop",
    # web_tab_id 每进程一个，随会话变化；这里固定即可
    "web_tab_id": str(uuid.uuid4()),
}

# 鉴权最小集（docs/05 §1.2）。顺序无关，只要名字齐。
REQUIRED_COOKIES = (
    "sessionid", "sessionid_ss", "sid_tt", "sid_guard", "uid_tt", "uid_tt_ss",
    "d_ticket", "ttwid", "s_v_web_id", "passport_csrf_token",
)

# ---------------------------------------------------------------- 模型映射
# 来源：/alice/slot/action_bar_v3/brief_list 的 active_switch_conf
#   menu_conf_v2.model_list.item_list[*]（零请求，从已有样本解出）
#   default_select_model_map: chat->0 / office->9 / creation->9
#   switch_memory_v2.default_select_model_by_mode_id: mode 1->0 / mode 3->9
# reasoning_effort 实测默认 "3"（docs/03 §3.1 conversation_init_ext）。
@dataclass(frozen=True)
class Mode:
    model_item_key: str
    mode_id: str            # conversation_init_ext.mode_id
    conversation_mode: int  # option.conversation_mode
    agent_mode: int
    reasoning_effort: int
    display: str


MODES: dict[str, Mode] = {
    # 对话（chat，mode_id=1）
    "doubao-fast":      Mode("0", "1", 1, 2, 3, "豆包 快速"),
    "doubao-expert":    Mode("3", "1", 1, 2, 3, "豆包 专家"),
    # 工作（office，mode_id=3）
    "doubao-auto":      Mode("9", "3", 3, 2, 3, "自动"),
    "doubao-turbo":     Mode("4", "3", 3, 2, 3, "豆包 2.1 Turbo"),
    "doubao-pro":       Mode("5", "3", 3, 2, 3, "豆包 2.1 Pro"),
    "doubao-2.1-lite":  Mode("seed-lite-7b", "3", 3, 2, 3, "豆包 2.1 Lite"),
}
DEFAULT_MODEL = "doubao-fast"
# 兼容 OpenAI 习惯名
MODEL_ALIASES = {
    "doubao": DEFAULT_MODEL,
    "doubao-pro": "doubao-pro",
    "doubao-seed": "doubao-auto",
    "auto": "doubao-auto",
    "fast": "doubao-fast",
    "expert": "doubao-expert",
}

# 主 bot（docs/03 §3）
BOT_ID = os.environ.get("DOUBAO_BOT_ID", "7338286299411103781")

# 风控里不能自动重试的错误码（docs/05 §3）
NO_RETRY_CODES = {710022004, 710012000}
RETRYABLE_CODES = {710022002}


class DoubaoError(Exception):
    """上游错误。code 是豆包侧错误码，kind 用于映射 HTTP 状态。"""

    def __init__(self, message: str, code=None, kind: str = "upstream"):
        super().__init__(message)
        self.code = code
        self.kind = kind  # auth | risk | rate | upstream | transport

    def to_http(self) -> int:
        return {
            "auth": 401,
            "risk": 503,
            "rate": 429,
            "upstream": 502,
            "transport": 502,
        }.get(self.kind, 500)

    @property
    def retryable(self) -> bool:
        if self.code in NO_RETRY_CODES:
            return False
        return self.kind == "rate" or self.code in RETRYABLE_CODES


def _classify(code, message: str) -> str:
    if code in (710012000,) or "user invalid" in (message or "").lower():
        return "auth"
    if code == 710022004:
        return "risk"
    if code in (710022002,):
        return "rate"
    return "upstream"


# ---------------------------------------------------------------- cookie / query
def build_cookie(raw: str | None = None) -> str:
    """把整串 cookie 裁到最小必需集；缺必需名就直接报错，好过被风控静默拒绝。"""
    raw = raw if raw is not None else os.environ.get("DOUBAO_COOKIE", "")
    pairs = {}
    for part in raw.split(";"):
        part = part.strip()
        if not part or "=" not in part:
            continue
        k, v = part.split("=", 1)
        pairs[k.strip()] = v.strip()
    missing = [k for k in REQUIRED_COOKIES if k not in pairs]
    if missing:
        raise DoubaoError(
            "cookie 缺少必需项: %s" % ", ".join(missing), kind="auth")
    return "; ".join("%s=%s" % (k, pairs[k]) for k in REQUIRED_COOKIES)


def build_query() -> str:
    return "&".join("%s=%s" % (k, quote(str(v), safe="")) for k, v in BASE_QUERY.items())


def resolve_model(name: str | None) -> tuple[str, Mode]:
    if not name:
        return DEFAULT_MODEL, MODES[DEFAULT_MODEL]
    key = name.strip().lower()
    if key in MODES:
        return key, MODES[key]
    if key in MODEL_ALIASES:
        canon = MODEL_ALIASES[key]
        return canon, MODES[canon]
    # 允许直接传 model_item_key
    for canon, m in MODES.items():
        if m.model_item_key == name:
            return canon, m
    raise DoubaoError("unknown model: %s" % name, kind="upstream")


# ---------------------------------------------------------------- 请求信封
def build_envelope(text: str, mode: Mode, conversation_id: str = "",
                   local_conv_id: str | None = None,
                   bot_id: str = BOT_ID) -> dict:
    """五段式信封（docs/03 §4）。所有 ext 值都是字符串。"""
    local_conv = local_conv_id or "local_%d" % (int(time.time() * 1000) * 1000)
    now_ms = int(time.time() * 1000)
    return {
        "client_meta": {
            "local_conversation_id": local_conv,
            "conversation_id": conversation_id or "",
            "bot_id": bot_id,
            "last_section_id": "",
            "last_message_index": None,
            "local_permissions": [
                {"permission_name": "ACCESS_COARSE_LOCATION", "status": 1},
                {"permission_name": "ACCESS_FINE_LOCATION", "status": 1},
                {"permission_name": "ACCESS_BACKGROUND_LOCATION", "status": 1},
            ],
        },
        "messages": [{
            "local_message_id": str(uuid.uuid1()),
            "content_block": [{
                "block_type": 10000,  # 文本块
                "content": {
                    "text_block": {"text": text, "icon_url": "",
                                   "icon_url_dark": "", "summary": ""},
                    "pc_event_block": "",
                },
                "block_id": str(uuid.uuid4()),
                "parent_id": "", "meta_info": [], "extra_info": {},
                "append_fields": [],
            }],
            "message_status": 0,
        }],
        "option": {
            "send_message_scene": "", "create_time_ms": now_ms,
            "collect_id": "", "is_audio": False, "answer_with_suggest": False,
            "agent_mode": mode.agent_mode, "tts_switch": False,
            "need_deep_think": 0, "click_clear_context": False,
            "from_suggest": False, "is_regen": False, "is_replace": False,
            "is_from_click_option": False, "is_from_click_softlink": False,
            "disable_sse_cache": False, "select_text_action": "",
            "is_select_text": False, "resend_for_regen": False, "scene_type": 0,
            "unique_key": str(uuid.uuid4()), "start_seq": 0,
            "need_create_conversation": not bool(conversation_id),
            "conversation_init_option": {"need_ack_conversation": True},
            "conversation_init_ext": {
                "model_item_key": mode.model_item_key,
                "reasoning_effort": str(mode.reasoning_effort),
                "mode_id": mode.mode_id,
            },
            "regen_query_id": [], "edit_query_id": [], "regen_instruction": "",
            "no_replace_for_regen": False, "message_from": 0,
            "shared_app_name": "", "shared_app_id": "",
            "sse_recv_event_options": {"support_chunk_delta": True},
            "support_lazy_fetch_stream": True,
            "is_ai_playground": False, "is_old_user": True,
            "general_task_param": {"server_do_compare": True},
            "recovery_option": {"is_recovery": False,
                                "req_create_time_sec": int(now_ms // 1000),
                                "append_sse_event_scene": 0},
            "message_storage_type": 0, "related_deleted_message_ids": {},
            "connector_info_list": [],
            "model_config": {"model_item_key": mode.model_item_key,
                             "model_extra_params": {},
                             "reasoning_effort": mode.reasoning_effort},
            "aggregate_params": {
                "mention_skill_list": "[]", "mention_plugin_list": "[]",
                "mention_ext": "[{}]",
                "conversation_mode": str(mode.conversation_mode),
                "mode_id": mode.mode_id,
                "model_item_key": mode.model_item_key,
                "agent_mode": str(mode.agent_mode),
                "reasoning_effort": str(mode.reasoning_effort),
                "provider_id": "",
            },
            "conversation_mode": mode.conversation_mode,
        },
        "user_context": [],
        "ext": {
            "agent_mode": str(mode.agent_mode),
            "use_deep_think": "0",
            "sub_conv_firstmet_type": "1",
            "general_task_param": json.dumps({"server_do_compare": True}),
            "collection_id": "",
            "is_finish": "1",
            "conversation_init_option": json.dumps({"need_ack_conversation": True}),
            "commerce_credit_config_enable": "0",
        },
    }


# ---------------------------------------------------------------- SSE 解析
@dataclass
class StreamState:
    conversation_id: str | None = None
    section_id: str | None = None
    question_id: str | None = None
    answer_message_id: str | None = None
    local_message_id: str | None = None
    text_parts: list[str] = field(default_factory=list)
    suggestions: list[str] = field(default_factory=list)
    brief: str | None = None
    model_type: str | None = None
    log_id: str | None = None
    channels: set = field(default_factory=set)

    @property
    def text(self) -> str:
        return "".join(self.text_parts)


def _norm(s: str) -> str:
    """比较用：折叠所有空白。brief 是预览串会吃换行，只有折叠后才可比。"""
    return "".join((s or "").split())


def _add_text(st: StreamState, piece: str, channel: str):
    """正文片段直接累加。

    实测两条正文通道（`CHUNK_DELTA` 与 `STREAM_CHUNK patch_object:1`）**互不重叠**：
    抓包里 patch(1) 给 "你"/"好"，delta 给 "呀！有什么我可以"/"帮你的吗？"；
    探针 P2 则只有 patch(1)、完全没有 delta。
    所以这里**不按内容去重**（按内容去重会误伤 "哈哈" 这类合法重复片段），
    末尾再用 `end_type:1` 的 `brief` 做权威校正。`tts_content`(patch_object 111)
    是镜像通道，解析时直接丢弃。
    """
    if not piece:
        return None
    st.channels.add(channel)
    st.text_parts.append(piece)
    return piece


def parse_sse_line(line: str, cur: dict):
    """把一行喂进 SSE 帧累加器，帧完整时返回 (event, data_obj)。"""
    line = line.rstrip("\r\n")
    if not line:
        ev = cur.get("event")
        data = cur.get("data")
        cur.clear()
        if not ev:
            return None
        try:
            obj = json.loads(data) if data else {}
        except Exception:
            obj = {"_raw": data}
        return ev, obj
    if line.startswith(":"):           # 注释行
        return None
    if line.startswith("event:"):
        cur["event"] = line[6:].strip()
    elif line.startswith("data:"):
        chunk = line[5:].strip()
        cur["data"] = (cur.get("data", "") + chunk) if cur.get("data") else chunk
    elif line.startswith("id:"):
        cur["id"] = line[3:].strip()
    return None


# ---------------------------------------------------------------- 上游客户端
class DoubaoClient:
    def __init__(self, cookie: str | None = None, timeout: int = 120):
        self.timeout = timeout
        self.cookie = build_cookie(cookie)
        self._ctx = ssl.create_default_context()
        self._lock = threading.Lock()  # 单账号串行，天然限频

    # -- 底层一次请求 ----------------------------------------------------
    def stream(self, text: str, mode_name: str = DEFAULT_MODEL,
               conversation_id: str = ""):
        """执行一次对话，逐个产出规整化事件：

        ("meta", StreamState)     SSE_ACK 之后（会话/消息 ID 已定）
        ("delta", str)            正文片段
        ("done", dict)            end_type:3，含 full_text/brief/suggestions
        出错抛 DoubaoError。
        """
        return self._stream_impl(text, mode_name, conversation_id)

    def _stream_impl(self, text, mode_name, conversation_id):
        # 锁在生成器体内获取：整个流期间独占，避免同一 cookie 并发触发过频
        with self._lock:
            yield from self._stream_locked(text, mode_name, conversation_id)

    def _stream_locked(self, text, mode_name, conversation_id):
        canon, mode = resolve_model(mode_name)
        envelope = build_envelope(text, mode, conversation_id=conversation_id)
        payload = json.dumps(envelope, ensure_ascii=False,
                             separators=(",", ":")).encode("utf-8")
        path = "%s?%s" % (COMPLETION_PATH, build_query())
        headers = {
            "content-type": "application/json",
            "agw-js-conv": "str",          # 抓包是 "str, str"，单值即可（docs/03 §3）
            "user-agent": USER_AGENT,
            "accept": "*/*",
            "accept-encoding": "identity",  # 免解压，SSE 直读
            "accept-language": "zh-CN,zh;q=0.9",
            "origin": "https://%s" % HOST,
            "referer": "https://%s/" % HOST,
            "cookie": self.cookie,
            "priority": "u=1, i",
        }

        conn = http.client.HTTPSConnection(HOST, context=self._ctx,
                                           timeout=self.timeout)
        st = StreamState()
        st.local_message_id = envelope["messages"][0]["local_message_id"]
        try:
            conn.request("POST", path, body=payload, headers=headers)
            resp = conn.getresponse()
            status = resp.status
            log_id = resp.getheader("x-tt-logid")
            st.log_id = log_id
            if status != 200:
                body = b""
                try:
                    body = resp.read()[:400]
                except Exception:
                    pass
                raise DoubaoError(
                    "HTTP %d %s" % (status, body.decode("utf-8", "replace")),
                    code=status, kind="auth" if status in (401, 403) else "upstream")

            cur: dict = {}
            end_stages = set()
            while True:
                try:
                    raw = resp.readline()
                except Exception as exc:
                    raise DoubaoError("stream interrupted: %s" % exc,
                                      kind="transport") from exc
                if not raw:
                    break
                parsed = parse_sse_line(raw.decode("utf-8", "replace"), cur)
                if not parsed:
                    continue
                ev, data = parsed
                out = self._handle_event(ev, data, st, end_stages)
                if out is None:
                    continue
                for item in out:
                    yield item
                if "final" in end_stages:
                    break
        finally:
            try:
                conn.close()
            except Exception:
                pass

    def _handle_event(self, ev: str, data: dict, st: StreamState, end_stages: set):
        if ev == "SSE_HEARTBEAT":
            return None

        if ev == "SSE_ACK":
            ack = data.get("ack_client_meta") or {}
            st.conversation_id = ack.get("conversation_id") or st.conversation_id
            st.section_id = ack.get("section_id") or st.section_id
            ql = data.get("query_list") or []
            if ql:
                st.question_id = ql[0].get("question_id")
            return [("meta", st)]

        if ev == "FULL_MSG_NOTIFY":
            # 用户自己那条落库；顺带抓服务端真实模型号
            msg = data.get("message") or {}
            mt = (msg.get("ext") or {}).get("model_type")
            if mt:
                st.model_type = str(mt)
            return None

        if ev == "STREAM_MSG_NOTIFY":
            content = data.get("content") or {}
            meta = data.get("meta") or {}
            if meta.get("message_id"):
                st.answer_message_id = meta["message_id"]
            mt = (content.get("ext") or {}).get("model_type")
            if mt:
                st.model_type = str(mt)
            pieces = []
            for blk in content.get("content_block") or []:
                t = (blk.get("content") or {}).get("text_block", {}).get("text")
                r = _add_text(st, t, "stream_msg_notify")
                if r:
                    pieces.append(("delta", r))
            return pieces or None

        if ev == "CHUNK_DELTA":
            t = data.get("text") or ""
            r = _add_text(st, t, "chunk_delta")
            return [("delta", r)] if r else None

        if ev == "STREAM_CHUNK":
            out = []
            for op in data.get("patch_op") or []:
                obj = op.get("patch_object")
                val = op.get("patch_value") or {}
                if obj == 1:            # 正文块
                    for blk in val.get("content_block") or []:
                        t = (blk.get("content") or {}).get("text_block", {}).get("text")
                        r = _add_text(st, t, "stream_chunk_patch1")
                        if r:
                            out.append(("delta", r))
                elif obj == 50:         # ext：建议、is_finish、brief
                    ext = val.get("ext") or {}
                    sp = ext.get("sp_v2")
                    if sp:
                        try:
                            st.suggestions = [s.get("content", "").strip()
                                              for s in json.loads(sp)]
                        except Exception:
                            pass
                elif obj == 111:        # tts_content 镜像，正文已从别处拿到
                    continue
            return out or None

        if ev == "SSE_REPLY_END":
            et = data.get("end_type")
            if et == 1:
                attr = data.get("msg_finish_attr") or {}
                st.brief = attr.get("brief")
                end_stages.add("body")
                # 正文此刻已完整：立刻通知下游可以出 finish_reason，
                # 不必等建议阶段（实测可省下 2~3 秒）
                return [("body_end", {"conversation_id": st.conversation_id,
                                      "question_id": st.question_id,
                                      "answer_message_id": st.answer_message_id,
                                      "model_type": st.model_type,
                                      "log_id": st.log_id})]
            elif et == 3:
                end_stages.add("final")
                assembled = st.text
                if st.brief is not None and _norm(st.brief) != _norm(assembled):
                    # brief 是会话列表预览串：实测会吃掉换行（'1\n2\n…12' → '123456789101112'），
                    # 长回答还可能截断。只作观测，绝不覆盖拼接出的正文。
                    log.debug("brief differs: assembled=%r brief=%r",
                              assembled, st.brief)
                return [("done", {"text": assembled,
                                  "brief": st.brief,
                                  "conversation_id": st.conversation_id,
                                  "question_id": st.question_id,
                                  "answer_message_id": st.answer_message_id,
                                  "model_type": st.model_type,
                                  "suggestions": st.suggestions,
                                  "channels": sorted(st.channels),
                                  "log_id": st.log_id})]
            return None

        if ev == "STREAM_ERROR":
            code = data.get("error_code")
            msg = data.get("error_msg") or "doubao stream error"
            extra = data.get("extra") or {}
            kind = _classify(code, msg)
            detail = msg
            dec = extra.get("decision")
            if dec:
                try:
                    d = json.loads(dec)
                    detail = "%s (风控要求验证: type=%s subtype=%s)" % (
                        msg, d.get("type"), d.get("subtype"))
                except Exception:
                    pass
            raise DoubaoError("%s [logid=%s]" % (detail, st.log_id),
                              code=code, kind=kind)

        if ev == "gateway-error":
            try:
                inner = json.loads(data.get("message") or "{}")
                code = inner.get("code")
                msg = inner.get("message") or str(data)
            except Exception:
                code, msg = None, str(data)
            raise DoubaoError("gateway-error: %s" % msg[:300],
                              code=code, kind=_classify(code, msg))

        log.debug("unhandled SSE event: %s", ev)
        return None

    # -- 非流式 ----------------------------------------------------------
    def complete(self, text: str, model: str = DEFAULT_MODEL,
                 conversation_id: str = "") -> dict:
        for kind, val in self.stream(text, model, conversation_id):
            if kind == "done":
                return val
        raise DoubaoError("stream ended without done", kind="transport")
