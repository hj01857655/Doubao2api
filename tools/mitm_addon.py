#!/usr/bin/env python3
# -*- coding: utf-8 -*-
r"""
Phase 0 · 豆包模型流量一次性取证 addon（mitmproxy）

落盘策略（2026-10-16 决定）：
  **原始字节完整落盘**到 tools/samples/，含真实请求头与 cookie —— 反代要复现请求，
  骨架化样本不够用（99 §1 的 btrans 线格式缺口需要原始字节，protobuf 无法从键路径还原）。
  防泄漏靠两道：
    1) tools/samples/.gitignore 内容为 `*`（目录自忽略，即使日后被纳入任何上层仓库也不入库）
    2) 项目根 .gitignore 再兜一层
  日常查阅只 grep manifest.jsonl（键路径骨架 + 头名清单），需要原始报文时才碰 .bin。

用法（PowerShell）—— **零参数即可启动全量抓包**：
  # 方式一（推荐）：直接跑本文件，自带默认参数 127.0.0.1:8080 + 加载自身为 addon
  .\.venv\Scripts\python.exe tools\mitm_addon.py
  # 方式二：显式起 mitmdump（与方式一等价）
  .\.venv\Scripts\mitmdump.exe -s tools\mitm_addon.py --listen-host 127.0.0.1 --listen-port 8080
  # 浏览器（独立 profile + 仅本实例代理）
  pwsh tools\run_browser.ps1

  注意：**必须用项目 .venv**。全局 Python 3.13 里 cryptography 50 与 pyOpenSSL 25.1 不匹配，
  import 阶段就炸 `AttributeError: module 'lib' has no attribute 'GEN_EMAIL'`。

判读（2026-10-16 已实测，见 ../README.md「Phase 0 判读」）：
  - MITM 对豆包 Web 完全可行（tls_error=0），对话通道是 wss100-normal.doubao.com/ws/v2 +
    www.doubao.com 的 cmd RPC 信封；未登录态只有匿名接口，正文需登录一次。
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import time

# 重点域名（DOUBAO-05/07 实测）——**2026-10-16 起只作标记，不再用于过滤**
# 上一版按目标列表过滤，结果整轮只看见 www.doubao.com 的 21 个 path，
# 对话正文若发往别的域名就完全不可见，连"否证"都判不了。故改为全量采集。
CORE_TARGETS = (
    "www.doubao.com",          # 主业务（Web 侧实测为 cmd RPC 信封，非 /top/v1）
    "keyhub.zijieapi.com",     # ttnet 握手
    "api-normal.doubao.com",   # ttnet 配置下发
)

_HOSTS_SEEN = set()

# 静态资源：manifest 只记一行（host/path/status/bytes），不落原始字节。
# 否则一轮页面加载就是几百 MB，且对协议还原零价值。
_ASSET_EXT = re.compile(
    r"\.(?:js|mjs|css|png|jpe?g|gif|webp|avif|svg|ico|woff2?|ttf|otf|eot|mp4|webm|mp3|m4a|wav|map|br|gz|zip)$",
    re.I,
)
_ASSET_CT = re.compile(r"^(image/|video/|audio/|font/|text/css|.*javascript)", re.I)

# 单个报文体落盘上限，超出截断并标记（防止大文件爆盘）
_MAX_RAW = 8 * 1024 * 1024

HERE = os.path.dirname(os.path.abspath(__file__))
SAMPLES = os.path.join(HERE, "samples")
LOG_PATH = os.path.join(HERE, "capture.log")
MANIFEST = os.path.join(SAMPLES, "manifest.jsonl")

# 自忽略：无论 samples/ 日后落在谁的仓库里，内容都不入库
SELF_IGNORE = os.path.join(SAMPLES, ".gitignore")

_SECRET_NAME = re.compile(
    r"^(authorization|cookie|set-cookie|x-tt-.*|x-.*-token|.*token.*|.*signature.*|x-lark-.*|x-device-.*|x-uid)$",
    re.I,
)
_BINARY_CT = re.compile(r"^(image|video|audio|font|application/(octet|protobuf|x-protobuf|gzip))", re.I)


def _ensure_dirs() -> None:
    os.makedirs(SAMPLES, exist_ok=True)
    if not os.path.exists(SELF_IGNORE):
        with open(SELF_IGNORE, "w", encoding="utf-8") as f:
            f.write("# 目录自忽略：抓包样本含真实凭证，永不入库\n*\n!.gitignore\n")


def _log(line: str) -> None:
    text = "[%s] %s" % (time.strftime("%H:%M:%S"), line)
    print(text, flush=True)
    with open(LOG_PATH, "a", encoding="utf-8") as f:
        f.write(text + "\n")


def _host_hit(host: str) -> bool:
    """全量采集：任何 host 都记。返回是否属于重点关注域（写进 manifest 的 core 标记）。"""
    return any(host == t or host.endswith("." + t) for t in CORE_TARGETS)


def _is_asset(path: str, content_type: str) -> bool:
    return bool(_ASSET_EXT.search(path) or _ASSET_CT.match(content_type or ""))


def _skeleton(obj, prefix: str = "", depth: int = 0) -> dict:
    """JSON → 键路径骨架（类型+长度）。原始值另有 .bin，骨架只为免开文件就能 grep。"""
    node = {}
    if depth > 5:
        return {prefix or "...": "max-depth"}
    if isinstance(obj, dict):
        for k, v in obj.items():
            p = f"{prefix}.{k}" if prefix else k
            if isinstance(v, (dict, list)):
                node.update(_skeleton(v, p, depth + 1))
            elif isinstance(v, str):
                node[p] = "str:%d" % len(v)
            else:
                node[p] = type(v).__name__
    elif isinstance(obj, list):
        node[prefix or "[]"] = "list:%d" % len(obj)
        if obj:
            node.update(_skeleton(obj[0], (prefix or "[]") + "[0]", depth + 1))
    return node


def _digest_headers(h) -> dict:
    """manifest 里只放头名 + 长度 + 指纹；完整值在 .bin 的 headers 段。"""
    out = {}
    for k, v in h.items():
        if _SECRET_NAME.match(k):
            out[k] = "SECRET(len=%d,sha1=%s)" % (len(v), hashlib.sha1(v.encode("utf-8", "replace")).hexdigest()[:8])
        else:
            out[k] = str(v)[:120]
    return out


def _dump_raw(tag: str, seq: str, path: str, headers: dict, body: bytes) -> str:
    """原始报文落盘：headers 明文 + body 原字节。这是复现请求所需的全部信息。"""
    name = "%s-%s.%s.bin" % (seq, re.sub(r"\W+", "_", path)[:40], tag)
    full = os.path.join(SAMPLES, name)
    if body and len(body) > _MAX_RAW:
        body = body[:_MAX_RAW] + b"\n=== TRUNCATED-AT-%dB ===\n" % _MAX_RAW
    with open(full, "wb") as f:
        f.write(b"=== HEADERS ===\n")
        for k, v in headers.items():
            f.write(("%s: %s\n" % (k, v)).encode("utf-8", "replace"))
        f.write(b"=== BODY ===\n")
        f.write(body or b"")
    return name


def _append_manifest(rec: dict) -> None:
    with open(MANIFEST, "a", encoding="utf-8") as f:
        f.write(json.dumps(rec, ensure_ascii=False, default=str) + "\n")


_counter = {"n": 0}


def _seq() -> str:
    _counter["n"] += 1
    return "%s-%03d" % (time.strftime("%H%M%S"), _counter["n"])


def _note_host(host: str) -> None:
    """首次见到某 host 就大声记一行 —— 全量采集时这是最省事的「正文发去哪」探针。"""
    if host in _HOSTS_SEEN:
        return
    _HOSTS_SEEN.add(host)
    _log("NEW_HOST %s   (core=%s)" % (host, _host_hit(host)))


def request(flow):
    try:
        host = flow.request.pretty_host
        _ensure_dirs()
        _note_host(host)
        seq = _seq()
        path = flow.request.path.split("?")[0]
        headers = dict(flow.request.headers.items())
        body = flow.request.get_content(strict=False) or b""
        ct = flow.request.headers.get("content-type", "")
        asset = _is_asset(path, ct)
        raw_name = None if asset else _dump_raw("req", seq, path, headers, body)

        rec = {
            "seq": seq,
            "ts": int(time.time()),
            "host": host,
            "core": _host_hit(host),
            "asset": asset,
            "method": flow.request.method,
            "path": path,
            "query_keys": sorted({kv.split("=")[0] for kv in flow.request.query}),
            "content_type": ct[:60],
            "body_bytes": len(body),
            "req_headers": _digest_headers(flow.request.headers),
            "raw_file": raw_name,
        }
        if body and "json" in ct:
            try:
                rec["body_skeleton"] = _skeleton(json.loads(body.decode("utf-8", "replace")))
            except Exception:
                pass
        elif body and _BINARY_CT.match(ct):
            printable = re.findall(rb"[ -~]{6,}", body)
            rec["body_anchor_strings"] = [p.decode("latin1")[:60] for p in printable[:20]]
        _append_manifest(rec)
        _log("REQ  %s %s%s | body=%dB -> %s" % (flow.request.method, host, path, len(body), raw_name))
    except Exception as exc:
        _log("request_error %s: %s" % (type(exc).__name__, exc))


def responseheaders(flow):
    try:
        if flow.request is None or flow.response is None:
            return
        host = flow.request.pretty_host
        _note_host(host)
        ct = flow.response.headers.get("content-type", "")
        path = flow.request.path.split("?")[0]
        _log("RESP %s %s status=%s ct=%s" % (host, path, flow.response.status_code, ct[:60]))
        if "event-stream" in ct:
            _log("     ^ SSE —— 模型流式响应，Phase 1 契约主战场")
        if flow.response.headers.get("upgrade", ""):
            _log("     ^ UPGRADE=%s —— 长连接（IM 通道可能走 websocket）"
                 % flow.response.headers["upgrade"])
    except Exception as exc:
        _log("resp_headers_error %s" % type(exc).__name__)


def response(flow):
    try:
        host = flow.request.pretty_host
        _ensure_dirs()
        path = flow.request.path.split("?")[0]
        headers = dict(flow.response.headers.items())
        try:
            body = flow.response.get_content(strict=False) or b""
        except Exception:
            body = b""
        ct = flow.response.headers.get("content-type", "")
        asset = _is_asset(path, ct)
        raw_name = None if asset else _dump_raw("resp", _seq(), path, headers, body)

        rec = {
            "ts": int(time.time()),
            "host": host,
            "core": _host_hit(host),
            "asset": asset,
            "path": path,
            "status": flow.response.status_code,
            "content_type": ct[:60],
            "body_bytes": len(body),
            "resp_headers": _digest_headers(flow.response.headers),
            "raw_file": raw_name,
            "kind": "response",
        }
        if "json" in ct and body:
            try:
                rec["body_skeleton"] = _skeleton(json.loads(body.decode("utf-8", "replace")))
            except Exception:
                pass
        elif body and "event-stream" in ct:
            frames = re.findall(rb"(?:^|\n)(event|data):[ -~]{4,120}", body[:200000])
            rec["sse_frame_sample"] = [f.decode("latin1")[:120] for f in frames[:12]]
        _append_manifest(rec)
        _log("     -> %s (%dB)" % (raw_name or "asset(no-raw)", len(body)))
    except Exception as exc:
        _log("response_error %s: %s" % (type(exc).__name__, exc))


def websocket_message(flow):
    """IM/正文通道走 ws —— mitmproxy 已把分片重组为单条消息。
    mitmproxy 12 的 WebSocketMessage 实测属性：`from_client`(bool) / `type`(Opcode) / `content` /
    `timestamp` / `dropped` / `injected` / `is_text` / `text`。
    **没有** `.direction`、**没有** `.opcode`、**没有** `.fin`（两次 AttributeError 都是我自己臆造字段）。
    且 mitmproxy 只暴露 TEXT/BINARY 组装消息，ping/pong/close 不进这里。
    """
    try:
        _ensure_dirs()
        msg = flow.websocket.messages[-1]
        payload = msg.content or b""
        seq = _seq()
        host = flow.request.pretty_host
        tag = "req" if msg.from_client else "resp"
        opcode = int(msg.type)
        raw_name = _dump_raw(
            tag, seq, "%s%s" % (host, flow.request.path or "/"),
            {"ws_frame": "from_client=%s opcode=%d is_text=%s len=%d ts=%s" % (
                msg.from_client, opcode, msg.is_text, len(payload), msg.timestamp)},
            payload)
        rec = {
            "seq": seq, "ts": int(time.time()), "host": host, "core": _host_hit(host),
            "kind": "websocket", "from_client": msg.from_client,
            "opcode": opcode, "is_text": bool(msg.is_text),
            "bytes": len(payload), "raw_file": raw_name,
        }
        if payload[:1] in (b"{", b"["):    # cmd RPC 若走 ws，明文 JSON 直接可见
            try:
                rec["body_skeleton"] = _skeleton(json.loads(payload.decode("utf-8", "replace")))
                rec["cmd"] = json.loads(payload.decode("utf-8", "replace")).get("cmd")
            except Exception:
                pass
        _append_manifest(rec)
        _log("WS   %s %s %s %dB -> %s" % (
            host, flow.request.path, "C>" if msg.from_client else "<S", len(payload), raw_name))
    except Exception as exc:
        _log("ws_error %s: %s" % (type(exc).__name__, exc))


def error(flow):
    """钉扎/证书失败走这里 —— 现在全 host 都记，否证证据一网打尽。"""
    try:
        if flow.request is None:
            return
        host = flow.request.pretty_host
        _ensure_dirs()
        _note_host(host)
        msg = str(getattr(flow, "error", None) or "unknown")
        _append_manifest({"ts": int(time.time()), "host": host, "core": _host_hit(host),
                          "kind": "tls_error", "error": msg[:300]})
        _log("TLS_OR_CONNECT_FAIL host=%s err=%s" % (host, msg[:200]))
    except Exception:
        pass


# ---------------------------------------------------------------------------
# 零参数启动：python tools/mitm_addon.py 即起代理并加载自身为 addon
# ---------------------------------------------------------------------------
LISTEN_HOST = os.environ.get("MITM_HOST", "127.0.0.1")
LISTEN_PORT = os.environ.get("MITM_PORT", "8080")


def _main() -> int:
    try:
        from mitmproxy.tools.main import mitmdump
    except ImportError:
        print("缺 mitmproxy —— 用项目 venv 跑：  .\\.venv\\Scripts\\python.exe tools\\mitm_addon.py",
              flush=True)
        return 1

    _ensure_dirs()
    print("=" * 68, flush=True)
    print("全量抓包已启动  proxy=http://%s:%s" % (LISTEN_HOST, LISTEN_PORT), flush=True)
    print("  manifest -> %s" % MANIFEST, flush=True)
    print("  样本(原始字节, 自忽略) -> %s" % SAMPLES, flush=True)
    print("  日志 -> %s" % LOG_PATH, flush=True)
    print("浏览器： pwsh tools\\run_browser.ps1", flush=True)
    print("Ctrl+C 停止", flush=True)
    print("=" * 68, flush=True)
    return mitmdump([
        "-s", os.path.abspath(__file__),
        "--listen-host", LISTEN_HOST,
        "--listen-port", LISTEN_PORT,
        # stream_large_bodies 留默认（不限流缓冲），SSE 与 ws 帧需要完整内容
        "-q",                                # 只留本 addon 的 _log 输出，不刷屏
    ]) or 0


if __name__ == "__main__":
    raise SystemExit(_main())
