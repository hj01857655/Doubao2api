# 08 btrans body 帧格式（静态还原）

主题：ttnet 私有字节传输层（btrans）的**帧结构与错误码体系**——即"加密包体"里的 body 帧。
证据：`app/sscronet.dll` strings（`workspace/native-network/str_sscronet.dll.txt`）中 `net\tt_net\bytetransport\frames\btrans_framer.cc` 及其邻接串。
实现文件：`btrans_framer.cc`（见 06 §3）。

## 1. 帧类型（8 种，字面量直接可见）

```
stream_frame   message_frame  receipt_frame  ping_frame
settings_frame endstream_frame goaway_frame  priority_frame
```

## 2. 帧字段

| 项 | 证据串 |
|---|---|
| 帧头 | `frame_type=` / `frame_flags=` / `public_payload_length=` / `last_stream_id=` / `error_code=` / `error_description=` |
| 消息帧 | `meta_size=` / `content_size=` / `padding_length=` / `need_ack:` / `end_message:` / `weight=` |
| settings 条目 | `entry_prefix == kUint32SettingsEntryPrefix` |

**结构判读**：`public_payload_length` + `meta_size` / `content_size` / `padding_length` 三元组
说明帧由 **公开载荷 + 加密元数据 + 密文内容** 构成——这正是"sscronet 内的 body 帧"的形态。

## 3. 装配约束（断言串，反映解析器状态机）

```
!partial_buffer_.empty()
!pending_public_fields_.has_value()
pending_public_fields_.has_value()
public_fields_consumed <= predicted_size
public_fields_consumed >= origin_buffered_bytes
bytes_consumed_from_data <= input->size()
appended_size <= data->size()
buffered
count <= size()
```

含义：公开字段可**先于**载荷到达（`pending_public_fields_` 为可选项），解析器按 `predicted_size`
对齐公开头与缓冲边界——即支持**流式分片重组**。

## 4. 控制流与心跳

| 域 | 证据 |
|---|---|
| 控制流 | `bt_control`、`SendAppBackgroundSettings`（`is_background:`）、`SendGoAway`（`goaway_error:` / `goaway_detail:`）、`AckGoAway` |
| 心跳 | `SendHeartbeat` / `ScheduleNextHeartbeat`（`heartbeat_max_failures > 0`） |
| 会话池 | `session_key:`、`session_config`、`RequestSession`、`OnJobComplete`、`ip_changed active:` / ` sessions:` |
| 流 ID | `btrans_stream_id_manager.cc` 的 `reserved` |
| 帧装配日志 | `frame_type=` / `, frame_flags=` / `, public_payload_length=` / `, last_stream_id=` / `, error_code=` / `, error_description=`；消息侧 `, weight=` / `, end_message: ` / `, need_ack: ` / `, padding_length=` / `, meta_size=` / `, content_size=` |

## 5. 传输错误码体系（30 余项 `BTRANS_*`）

| 类别 | 码 |
|---|---|
| 加密/编解码 | **`BTRANS_DECRYPT_FAIL`**、`BTRANS_META_CODEC_FAIL` |
| 认证/风控 | `BTRANS_AUTH_FAIL`、`BTRANS_WAF_MSSDK_CHECK_FAIL`、`BTRANS_BY_BACKSERVICE` |
| 握手/TLS | `BTRANS_TLS_HANDSHAKE`、`BTRANS_HANDSHAKE_TIMEOUT` |
| 会话/连接 | `BTRANS_SESSION_IDLE_TIMEOUT`、`BTRANS_CONN_REUSE_FAIL`、`BTRANS_CONN_CLOSED`、`BTRANS_CONN_HIJACKED`、`BTRANS_CONN_NO_LISTEN`、`BTRANS_CONN_IDLE_TIMEOUT`、`BTRANS_EPOLL_CLOSED`、`BTRANS_TOO_MANY_CONNECTIONS`、`BTRANS_REJECT_NEW_SESSION` |
| 流/协议 | `BTRANS_PROTOCOL_ERROR`、`BTRANS_INVALID_STATE`、`BTRANS_UNSUPPORTED`、`BTRANS_TOO_MANY_STREAMS`、`BTRANS_NO_AVAILABLE_STREAM`、`BTRANS_DATA_TOO_LARGE`、`BTRANS_UNKNOWN_SETTINGS`、`BTRANS_SETTINGS_ERROR`、`BTRANS_REPEATED_CONTROL_STREAM`、`BTRANS_QUIC_ERROR` |
| 关闭语义 | `BTRANS_NORMAL_CLOSURE`、`BTRANS_REMOTE_CLOSURE`、`BTRANS_ABNORMAL_CLOSURE`、`BTRANS_GOING_AWAY`、`BTRANS_END_STREAM`、`BTRANS_CANCEL`、`BTRANS_SERVER_RESTART`、`BTRANS_TRY_AGAIN_LATER`、`BTRANS_INTERNAL_ERROR`、`BTRANS_BIZ_ERROR` |
| 读写超时 | `BTRANS_READ_TIMEOUT`、`BTRANS_WRITE_TIMEOUT`、`BTRANS_PING_TIMEOUT`、`BTRANS_PING_WRITE_FAIL`、`BTRANS_DATA_WRITE_ERROR` |

## 6. 与上层的关系

```
frontierx（HTTP/2-3 私有前沿层，19 模块）
   └─ btrans（字节传输：帧 / 流 / 会话池 / 自有 TLS）
         └─ tt_encrypt（encrypt_config_v1 + bk_k/bk_t，见 07）
```

- **日志格式实证**：`Cronet/TTNetVersion:`、`ttnet_version`、`x-ttnet-L`、`TTNet-TPH`、`OnTTNetDetectFinish`
  说明 ttnet 在 User-Agent/探测头里自报版本。
- **QUIC 提示**：`ttnet_quic_enabled` / `ttnet_quic_hint`（见 07 §2）启用 HTTP/3 路径。

## 7. 剩余工作

字节级布局（各字段**宽度与顺序**）需 `btrans_framer.cc` 的单函数反编译 ——
受 06 §2 的 rea 体量限制，需换 Hopper/IDA。