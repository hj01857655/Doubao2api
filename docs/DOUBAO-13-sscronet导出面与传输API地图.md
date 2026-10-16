# 13 sscronet 导出面与传输 API 地图

主题：`app/sscronet.dll` 的**导出符号面**（1381 个具名导出）——ttnet 私有层暴露给宿主的 C API。
证据：`pefile` 解析导出表 + PE 结构；原始导出清单落盘 `workspace/native-network/sscronet_exports.txt`（1381 行）。
本篇与 [06 私有模块地图](DOUBAO-06-ttnet私有模块地图.md) 互补：06 从**编译期源码路径**还原模块清单，本篇从**导出表**还原对外 API 面。

## 1. PE 结构与构建线索（实测）

| 项 | 值 |
|---|---|
| 体量 | 9,855,888 字节（9.86MB） |
| 机器 | `0x8664`（PE32+ x64） |
| ImageBase | `0x180000000` |
| 关键节 | `.text` va=0x1000 vsize=0x7C8995 ／ `.rdata` va=0x7CA000 vsize=0x1371C4 |
| 调试目录 | CodeView（type 2）→ `sscronet.dll.pdb` age 1（**未内嵌**，无本地符号） |

> 结论：**私有层以 C ABI 导出**（非隐藏符号），因此即使无 PDB，导出名本身就是一层 API 地图。

## 2. 导出家族总览

| 家族 | 数量 | 定性 |
|---|---|---|
| `FTX_*` | 164 | **frontierx 传输 SDK**（独立 C API，见 §3） |
| `Cronet_*` | 1170 | Cronet 面（含 ttnet 私有参数，见 §5） |
| `Telemetry_*` | 16 | 上报/埋点客户端 |
| `bidirectional_stream_*` | 9 | cronet 标准双向流 C API（btrans 上层） |
| `TT_CustomThread_*` | 6 | 自定义线程钩子 |
| C++ 构造/析构 | 16 | `??0/??1` 修饰名（`Cronet_FrontierConfig` 等） |

`Cronet_*` 内主要子族（按第二段）：

| 子族 | 数量 | 子族 | 数量 |
|---|---|---|---|
| EngineParams | 68 | UrlRequestParams | 40 |
| Engine | 62 | RTTAndThroughput | 20 |
| TTNetBasicRequestInfo | 56 | FrontierMessage | 22 |
| WSClientConnectionParams | 56 | ClientOpaqueData | 22 |
| TTNetParams | 52 | VerifyParamsV2 | 22 |
| FrontierConfig | 46 | Metrics / UrlResponseInfo | 40 / 24 |

## 3. `FTX_*` = frontierx 传输 SDK（164 导出）

06 §4 只能从 `.cc` 路径推断 frontierx 是"私有前沿层"；导出表显示它其实是**一整套对外的通道化消息传输 SDK**。结构：

| 层 | 对象 | 关键导出 |
|---|---|---|
| 客户端 | `FTX_Client` | `Create` / `Destroy` |
| 引擎 | `FTX_Engine` | `Create` / `Start` / `GetCronetEngine` / `SetCustomThread`；`EngineParams_EngineUniqueName/ThreadMode/WaitCronetInitTimeoutMs` |
| 连接 | `FTX_Connection*` | `Connection`/`ConnectionDest`/`ConnectionOpenParams`/`ConnectionInfo`/`ConnectionCallback` |
| 通道 | `FTX_Channel*` | `Channel_Open` / `SendMessage` / `GetProperty` / `InitCallback` / `Close` |
| 消息 | `FTX_Message` / `FTX_CustomMessage` | 消息与自定义消息构造 |
| 端点 | `FTX_IPEndPoint` | IP 端点 |

**客户端初始化参数**（`FTX_ClientInitParams_*`，决定它接哪套 ttnet）：

```
AppId / AppKey / AppChannel / AppVersion / AppUpdateVersion
DeviceId / DevicePlatform / FpId / Sampling / Tag
```

**通道打开参数**（`FTX_ChannelOpenParams_*`）——这是 frontierx 通道语义的核心：

```
Path / Query / Headers(Add) / Priority / Reliable / Directional / DispatchMode / Tag
```

**通道回调**（`FTX_ChannelCallback_*`）：`OnChannelOK` / `OnChannelError` / `OnRecvMessage` / `OnStreamSendStateUpdated`。
**通道属性**（`FTX_ChannelProperty_*`）：`ChannelId` / `Directional` / `FromRemote` / `IsReliable` / `Priority` / `TransportStreamId`。

> 定性：frontierx = **在 cronet 引擎之上、以"通道(Channel)"为单位的双向消息传输**；每条通道可声明可靠/方向/优先级/分发模式，
> 底层落到 `TransportStreamId`（即 btrans 流，见 06 §3 / 08）。`ClientInitParams` 里的 `AppId/AppKey/FpId` 说明它自带一套**应用级鉴权身份**。

## 4. `Cronet_Frontier*` = frontierx 的 Cronet 适配面（148 导出）

| 子族 | 数量 | 作用 |
|---|---|---|
| `Cronet_FrontierConfig` | 46 | 配置项（对 07 §4 的 30 项运行配置，走 C API 暴露） |
| `Cronet_FrontierMessage` / `MessageHeader` / `MessageHandler` | 22 / 6 / 10 | 消息体、消息头、处理器 |
| `Cronet_FrontierClient` / `ClientSDK` | 20 / 18 | 客户端与 SDK 门面（`Create`/`Configure`/`InitWithParams`/`Register`/`IsServiceReady`） |
| `Cronet_FrontierDelegate` / `SDKDelegate` | 10 / 5 | 回调委托 |
| `Cronet_FrontierParams` | 8 | 参数包 |

## 5. ttnet 私有参数面（藏在 Cronet 命名下）

Cronet 标准 API 之外，ttnet 私有的运行参数也以 `Cronet_*` 导出，是 07 §4 明文配置的**代码侧对应物**：

- **`Cronet_TTNetParams_*`（52）**：`app_id`/`app_name`/`channel`/`device_id`/`device_type`/`device_platform`/`uuid`/`version_code`/`update_version_code`、
  `domain_boe`/`domain_httpdns`/`domain_netlog`/`get_domain_default_json`、
  **`tnc_host_first/second/third` + `tnc_headers` + `tnc_queries`**、`tnc_load_flags`、`httpdns_load_flags`、`is_drop_first_tnc`、`is_main_process`、`use_domestic_store_region`。
- **`Cronet_WSClientConnectionParams_*`（56）**：WebSocket 连接参数（ttnet 私有）。
- **`Cronet_TTNetBasicRequestInfo_*`（56）**：请求基础信息（可用于对齐 07 的 `x-tt-e`/`x-tt-s` 头族来源）。
- `Cronet_ClientOpaqueData_private_key_*`（22）：客户端不透明数据携带的**私钥槽位**——与加密链（07）相关。

> TNC = 字节下发的网络配置（`tnc_host_*` + `tnc_queries` + `is_drop_first_tnc`），正好解释 06 §3 的 `btrans_tnc_configurer.cc` / `frontier_tnc_configurer.cc`。

## 6. 加密/帧层函数锚点（精确 RVA，供反汇编定位）

本篇用 PE 投影把 06 §6 的"源码路径"落成**可寻址 RVA**（ImageBase `0x180000000`）：

| 锚点字符串 | 文件偏移 | RVA | 备注 |
|---|---|---|---|
| `tt_encrypt_key_manager.cc` | 0x7F2F4F | **0x7F414F** | 加密配置/密钥管理（07 的 `bk_k` 派生位置） |
| `tt_encrypt_util.cc` | 0x81ECA3 | **0x81FEA3** | 加密工具 + `TokenManager` |
| `btrans_framer.cc` | 0x8205C7 | **0x8217C7** | 帧编解码（08 的实现位置） |

同 DLL 内出现的算法族字符串（实测计数）：`ECDH`×36、`HKDF`×3、`X25519`×2、`P-256`×3。
（`token_challenge` / `keyhub` 在本 DLL **零命中**——二者在别处，见 07。）

> 这些锚点是 `__FILE__` 宏字符串，非函数名；要定位承载函数需对 `.text` 做 RIP 相对引用扫描（见 §7 工具边界）。

## 7. 工具边界更新（对照 06 §2）

06 §2 记录 rea 对 9.86MB 的 `sscronet.dll` 两次 330s `provider_timeout`。本机**另有可用工具链**，不依赖 rea：

| 工具 | 用途 | 实测 |
|---|---|---|
| `pefile`（Python） | PE 结构 / 导出表 / 节投影 / 偏移↔RVA | ✅ 秒级 |
| `capstone`（Python） | 定向反汇编（按 RVA 取字节） | ✅ 可用 |
| `frida`（Python + CLI） | 动态 hook 导出函数（如抓 `FTX_Channel_SendMessage`） | ✅ 已装 |
| `dumpbin`（VS2022） | 导出/依赖/反汇编（`/DISASM /EXPORTS`） | ✅ 已装 |

> **意义**：私有层有**公开 C API** → 传输/帧/加密链可走"**API 层静态 + frida 动态**"两条路，不必先啃全量反编译。这比 06 §2 的"只能取 strings"是实质跃升。

## 一句话结论

`sscronet.dll` 以 **1381 个 C ABI 导出**把 ttnet 私有层摆到了明面上：**frontierx 是一套完整的通道化消息传输 SDK（164 导出）**，
Cronet 面之下藏着 ttnet 私有参数（TNC 下发、WS 连接、不透明数据私钥槽）与 frontierx 配置；
加密/帧实现位置有**可寻址 RVA 锚点**，配合 `capstone`/`frida`/`dumpbin` 即可开展单函数分析——rea 超时不再是阻断项。