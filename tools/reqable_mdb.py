#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""读取 Reqable 抓包库（ObjectBox 底层就是 LMDB）。

为什么要这个脚本
----------------
Reqable 的抓包记录分两处落盘，读的时候容易找错地方：

* ``%APPDATA%\\Reqable\\capture\\*.reqable`` —— **只有 body**，
  命名 ``<微秒时间戳>-<会话idx>-<部件>-<kind>``（``req_raw-body`` / ``res-raw-body``
  / ``res-extract-body`` / ``websocket``）。
* ``%APPDATA%\\Reqable\\box\\data.mdb`` —— **请求行、query、请求头、响应头、状态行**。
  头和体按「会话 idx + 时间戳」对应，不靠文件名里带 URL。

库结构（本机实测）
------------------
* LMDB：``data.mdb`` + ``lock.mdb``，**打开的是目录本身**，不是 .mdb 文件。
* 默认（无名）db 装全部记录（本机 1047 条）；抓包会话是 key 前 4 字节为
  ``18 00 00 2c`` 的那一组（本机 480 条），key = 该前缀 + 4 字节大端 id。
* 值 = flatbuffer 包着一段 **base64(gzip(JSON))**；正则抠出 base64 段再
  gunzip 就是可读 JSON：
  ``session.request.requestLine``（method / path=完整 URL+query / protocol）、
  ``session.request.headers[]``、``session.response.statusLine`` / ``headers[]``、
  ``session.connection.originHost`` / ``originPort`` / ``security``、
  ``session.timestamp``（微秒）。

用法
----
    python reqable_mdb.py list                      # 全部会话：时间/host/方法/状态
    python reqable_mdb.py list --grep doubao        # 只列 host 或 path 命中的
    python reqable_mdb.py show --id 365             # 某条会话的 query+头（默认脱敏）
    python reqable_mdb.py show --id 365 --raw       # 保留原值（含 Cookie/Token，谨慎）
    python reqable_mdb.py dump --out sessions.txt   # 全部会话脱敏导出

依赖：``pip install lmdb``（仅标准库 + lmdb）。

脱敏纪律（默认行为，遵循 Doubao2api README §安全与合规约束）
------------------------------------------------------------
``Cookie`` / ``Authorization`` / ``Set-Cookie`` / ``x-tt-*`` / ``x-ms-token`` /
``a_bogus`` / ``msToken`` / ``s_v_web_id`` / ``ttwid`` / ``web_id`` / ``web_tab_id``
/ ``tea_uuid`` / ``device_id`` / ``fp`` 一律只留**头名与长度**，值不落盘。
需要原值时显式加 ``--raw``，并自行承担后果。

Reqable 在运行中也能读：``readonly=True, lock=False`` 拿的是最后一次提交的一致快照，
不会干扰录制，也不会拿到写锁。
"""

from __future__ import annotations

import argparse
import base64
import datetime
import gzip
import json
import os
import re
import sys

DEFAULT_BOX = os.path.join(
    os.environ.get('APPDATA', ''), 'Reqable', 'box'
)

# 抓包会话记录的 key 前缀（本机实测；其余前缀是收藏夹/规则/环境等其他实体）
CAPTURE_KEY_PREFIX = b'\x18\x00\x00\x2c'

_B64_RE = re.compile(rb'[A-Za-z0-9+/]{40,}={0,2}')

# 只留名与长度的敏感头/参数
_SENSITIVE = re.compile(
    r'^(cookie|set-cookie|authorization|x-tt-|x-ms-token|mstoken|a_bogus|x-bogus'
    r'|s_v_web_id|ttwid|web_id|web_tab_id|tea_uuid|device_id|fp|.*token.*)$',
    re.I,
)


def _decode_value(value: bytes) -> bytes | None:
    """从 flatbuffer 值的 base64 段还原 gzip(JSON)；取最长的可解段。"""
    best = None
    for match in _B64_RE.finditer(value):
        candidate = match.group()
        try:
            raw = base64.b64decode(candidate + b'=' * (-len(candidate) % 4))
            data = gzip.decompress(raw)
        except Exception:
            continue
        if best is None or len(data) > len(best):
            best = data
    return best


def iter_sessions(box: str):
    """按 key 顺序产出 (key, 记录 dict)，只含可解码的抓包会话。"""
    import lmdb  # 延迟导入，便于在没有依赖时给出清晰报错

    with lmdb.open(box, readonly=True, lock=False, readahead=False) as env:
        with env.begin() as txn:
            for key, value in txn.cursor().iternext(keys=True, values=True):
                if key[:4] != CAPTURE_KEY_PREFIX:
                    continue
                data = _decode_value(value)
                if data is None:
                    continue
                try:
                    yield key, json.loads(data.decode('utf-8', 'replace'))
                except Exception:
                    continue


def _record_view(record: dict) -> dict:
    session = record.get('session') or {}
    conn = session.get('connection') or {}
    req = session.get('request') or {}
    resp = session.get('response') or {}
    line = req.get('requestLine') or {}
    return {
        'id': record.get('id'),
        'uid': record.get('uid'),
        'origin': record.get('origin'),
        'ts': int(session.get('timestamp') or 0),
        'host': conn.get('originHost') or '',
        'port': conn.get('originPort') or '',
        'security': conn.get('security'),
        'method': line.get('method') or '',
        'path': line.get('path') or '',
        'protocol': line.get('protocol') or '',
        'req_headers': req.get('headers') or [],
        'resp_headers': resp.get('headers') or [],
        'status': resp.get('statusLine'),
    }


def _fmt_time(micros: int) -> str:
    if not micros:
        return '?' * 23
    return datetime.datetime.fromtimestamp(micros / 1_000_000).strftime('%Y-%m-%d %H:%M:%S.%f')[:-3]


def _redact_header(header: str) -> str:
    if ':' not in header:
        return header[:300]
    name, _, value = header.partition(':')
    name, value = name.strip(), value.strip()
    if _SENSITIVE.match(name):
        return f'{name}: <redacted len={len(value)}>'
    return header[:300]


def _redact_query(path: str) -> str:
    base, sep, query = path.partition('?')
    if not sep:
        return base
    parts = []
    for pair in query.split('&'):
        if '=' in pair:
            key, _, value = pair.partition('=')
            parts.append(f'{key}=<len={len(value)}>' if _SENSITIVE.match(key) else pair)
        elif pair:
            parts.append(pair)
    return base + '?' + '&'.join(parts)


def _format_session(view: dict, raw: bool) -> str:
    path = view['path'] if raw else _redact_query(view['path'])
    headers = view['req_headers'] if raw else [_redact_header(h) for h in view['req_headers']]
    resp_headers = view['resp_headers'] if raw else [_redact_header(h) for h in view['resp_headers']]

    out = [
        '=' * 72,
        f"时间  : {_fmt_time(view['ts'])}",
        f"记录  : id={view['id']} uid={view['uid']} origin={view['origin']}",
        f"连接  : {view['host']}:{view['port']} security={view['security']}",
        f"请求行: {view['method']} {path} {view['protocol']}",
        '',
        f"请求头 ({len(headers)} 条):",
    ]
    out += ['   ' + h for h in headers]
    out += ['', '响应:', f"   statusLine: {view['status']}", f"   响应头 ({len(resp_headers)} 条):"]
    out += ['     ' + h for h in resp_headers]
    out.append('')
    return '\n'.join(out)


def cmd_list(args) -> int:
    rows = []
    for _key, record in iter_sessions(args.box):
        view = _record_view(record)
        if args.grep:
            needle = args.grep.lower()
            if needle not in view['host'].lower() and needle not in view['path'].lower():
                continue
        rows.append(view)
    rows.sort(key=lambda v: v['ts'])
    for view in rows:
        path = view['path'].split('?')[0]
        print(f"{_fmt_time(view['ts'])}  {view['method']:>7}  {view['host']}{path[:110]}")
    print(f'--- {len(rows)} 条会话 ---')
    return 0


def cmd_show(args) -> int:
    for _key, record in iter_sessions(args.box):
        view = _record_view(record)
        if view['id'] != args.id:
            continue
        print(_format_session(view, args.raw))
        return 0
    print(f'未找到 id={args.id} 的会话', file=sys.stderr)
    return 1


def cmd_dump(args) -> int:
    chunks = []
    count = 0
    for _key, record in iter_sessions(args.box):
        chunks.append(_format_session(_record_view(record), args.raw))
        count += 1
    with open(args.out, 'w', encoding='utf-8') as handle:
        handle.write('\n'.join(chunks))
    print(f'{count} 条会话 -> {args.out}')
    return 0


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description='读取 Reqable 抓包库（LMDB）')
    parser.add_argument('--box', default=DEFAULT_BOX, help=f'box 目录，默认 {DEFAULT_BOX}')
    parser.add_argument('--raw', action='store_true',
                        help='保留敏感值原值（默认脱敏：只留头名与长度）')
    sub = parser.add_subparsers(dest='command', required=True)

    p_list = sub.add_parser('list', help='列出会话')
    p_list.add_argument('--grep', default='', help='按 host 或 path 关键字过滤')
    p_list.set_defaults(func=cmd_list)

    p_show = sub.add_parser('show', help='打印一条会话的完整头面')
    p_show.add_argument('--id', type=int, required=True, help='会话 id（list 第二列可见）')
    p_show.set_defaults(func=cmd_show)

    p_dump = sub.add_parser('dump', help='导出全部会话')
    p_dump.add_argument('--out', required=True, help='输出文件')
    p_dump.set_defaults(func=cmd_dump)

    args = parser.parse_args(argv)
    if not os.path.isdir(args.box):
        print(f'box 目录不存在: {args.box}', file=sys.stderr)
        return 2
    return args.func(args)


if __name__ == '__main__':
    raise SystemExit(main())
