# 06 ttnet 私有模块地图与工具边界

主题：豆包原生网络栈（ttnet，cronet fork）**私有层**的完整模块地图。
证据：`app/sscronet.dll` strings 提取（31,895 条，`workspace/native-network/str_sscronet.dll.txt`）。

## 1. 模块面

| 模块 | 大小 | 定性 |
|---|---|---|
| `app/sscronet.dll` | **9,855,888 字节** | ttnet 网络栈本体（cronet fork + ttnet 私有层），本篇主证据源 |
| `app/aha_net.dll` | 2.4MB | aha 平台网络封装；strings 中**无** keyhub/handshake/tt_encrypt/encrypt_config（已排除） |
| `app/Doubao.dll` | 322,524,048 字节 | Chromium 壳 + 业务原生层 |

## 2. 工具边界（实测，重要）

`rea analyze sscronet.dll`：全量分析与 `--snapshot` 各一次，**均在 330s 处 `provider_timeout`**
（`{"operation":"list_documents","timeout_ms":330000}`）；Ghidra 后端（java.exe 峰值 1.1GB）确实在跑但超出 CLI 上限。

**结论**：该体量下 rea 的 analyze 不可用 → 符号面证据改由 strings 层获取（成本低、覆盖全）；
**单函数反编译需换 Hopper/IDA 等支持增量分析的后端**。尝试记录：`workspace/native-network/sscronet_analyze{,2}.json`。

## 3. `bytetransport/` 字节传输层（16 模块）

| 模块 | 职能 |
|---|---|
| `btrans_framer.cc` | **帧编解码**（body 帧格式实现位置，见 08） |
| `btrans_tls_connection.cc` / `btrans_tls_reliable_transport.cc` | btrans 自有 TLS 连接与可靠传输 |
| `btrans_session.cc` / `btrans_session_pool.cc` / `btrans_session_pool_job.cc` | 会话与连接池（`session_key:` 日志、`RequestSession`） |
| `btrans_stream.cc` / `btrans_datagram_stream.cc` / `btrans_message_stream.cc` / `btrans_control_stream.cc` | 四类流：普通 / 数据报 / 消息 / 控制 |
| `btrans_data_reader.cc` / `btrans_stream_id_manager.cc` | 读缓冲与流 ID 分配（stream_id 含 `reserved`） |
| `btrans_heartbeat_scheduler.cc` | 心跳（`heartbeat_max_failures > 0`） |
| `btrans_session_request.cc` / `btrans_transport_interface.cc` / `btrans_tnc_configurer.cc` | 请求对象、传输接口、TNC 下发配置装载 |

## 4. `frontierx/` 私有前沿层（19 模块）

`frontier_client_sdk.cc`、`frontier_session{,_factory,_factory_job,_solo_job}.cc`、`frontier_stream.cc`、
`frontierx_channel.cc`、`frontierx_connection.cc`、`frontier_btrans_session.cc`、`frontier_btrans_stream.cc`、
**`frontier_meta_decoder.cc`（元数据解码器）**、`frontier_tnc_configurer.cc`、`frontier_detect_manager.cc`、
`frontier_qos_manager.cc`、`frontier_qos_mmap_file.cc`、`frontier_rtt_prober.cc`、`frontier_client_delegate.cc`、
`frontier_connection_poll_metrics.cc`。

## 5. 其余私有域

| 路径 | 域 |
|---|---|
| `net\tt_net\encrypt\tt_encrypt_key_manager.cc` | **加密配置/密钥管理（唯一 encrypt 模块）** |
| `net\tt_net\netlog\tt_encrypt_util.cc` | 加密工具 + `TokenManager`（断言 ` key is null`），紧邻 boringssl `ecdsa_asn1.c` / `hkdf.c` / `ecdh_extra.c` |
| `net\tt_net\tudp\bis_connection.cc` | 字节 UDP（bis）连接 |
| `net\tt_net\net_detect\*` | 探测（traceroute / pinger / 质量 / 离线检测；`tnc_version=`、`&tnc_probe=`） |
| `net\tt_net\url_dispatcher\route_selection_v2\*` | 域名调度与选路（`race_context.cc`、`result_persistence.cc`、`detect_task.cc`） |
| `net\tt_net\cronet_frontierx_{channel,connection}.cc` | cronet 与 frontierx 适配层 |
| `net\third_party\quiche\src\quiche\spdy\*` | 第三方 quiche（HTTP/3） |

## 6. 方法论：源码路径即地图

sscronet.dll 保留了完整编译期源码相对路径（`..\..\net\tt_net\...`），因此**仅凭 strings 就能还原私有模块清单** ——
在 9.9MB 体量下 rea/Ghidra 不可用（§2）时，这是最划算的替代路径。

密钥派生实现位置由此锁定：`tt_encrypt_key_manager.cc` + `tt_encrypt_util.cc`（算法族与配置见 07）。