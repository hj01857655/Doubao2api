# 99 未深挖与后续

## 1. 剩余硬骨头（ttnet 加密链，唯一有难度的部分）

| 项 | 现状 | 下一步 |
|---|---|---|
| 握手响应格式 | `{version, random, key_share:{curve,pubkey}, cipher_suite, ticket, expire_time}`（07 §7） | — |
| `bk_k`→会话密钥派生 | keyhub 握手：ECDH(`secp256r1`) → ChaCha20(key, nonce)；函数 `0x284890`/`0x284eb0`（07 §7） | 逐字节 KDF 拼接未定位 |
| body 帧字节布局 | 帧类型/字段/错误码（08）+ 帧结构字段（15 §5） | 线格式字节编码（序列化体）未定位 |

**工具边界（实测）**：`rea analyze` 对 9.86MB 的 `sscronet.dll` 两次均 330s `provider_timeout`（`--snapshot` 同样，06 §2）；
**但本机工具链**（`pefile` / `capstone` / `frida` / `dumpbin`，见 13 §7）**不依赖 rea**——
已用其完成导出面解析（13）与**加密/帧函数定位 + 反汇编**（15，方法 = 功能串 xref）。

## 2. 未深挖的原生域 → **本轮已收口（见 14）**

| 区域 | 路径 | 结论 |
|---|---|---|
| 启动器 | `app/launcher/` | = 字节 **Saman Installer**（2.32.5 + 内嵌 RSA 更新签名，14 §1） |
| 终端 | `app/terminal_conpty/` | = **微软 ConPTY**（14 §2） |
| 端侧模型 | `app/model/`、`app/AUDIO_MODEL/` | 人脸/抠像/肤色/AEC/小型模型（14 §3） |
| 音视频资源 | `app/effects/`、`app/RTCSDK_RES/` | shader 效果包 + RTC 音频模型（14 §4） |
| 包管理 | `app/pip/` | `pip_main.exe` 嵌入式 Python 包管理（14 §5） |
| SDK 集 | `app/sdks/` | 仅 `aha_cua`（已在 09 成篇） |
| 词典/本地化 | `app/Dictionaries/`、`app/locales/` | 空 / Chromium 语言包（无逆向价值，14 §8） |
| 扩展面内部协议 | `local_webcontents/…` | 结构 + `env.json` SCM 版本（14 §6；消息面见 05 §5） |

**仍开放**：

| 区域 | 路径 | 说明 |
|---|---|---|
| `Doubao.dll` 原生主逻辑 | `app/Doubao.dll`（322MB） | 需 IDA/BinaryNinja 级分析；纯静态字串收益低 |
| 账户/token 存储 | `User Data/` 下 | **涉隐私，本次未动**（仅取了 `ahanet/` 网络配置） |

## 3. 可复用点（对 2api / 反代项目的启发）

| 机制 | 出处 | 可复用点 |
|---|---|---|
| **GAC 协议** | 03 | 二进制帧 + module id + ChaCha20-Poly1305 + 三凭证会话，是"让云端模型安全操作本机"的完整参考实现——比 MCP stdio 多了加密与执行会话绑定 |
| **技能下发体系** | 04 | manifest + checksum + 云端版本化，比本地目录扫描多一层供应链控制 |
| **`interaction.request_action` user-takeover 门** | 05 §4 | 把"需要人工"做成协议级动作而非聊天文本，杜绝模型自作主张 |
| **ttnet 配置面** | 07 | 分流/预连接/QUIC/HTTPDNS 表/重试策略全在明文配置里——网关类项目可对照其取舍（压缩按路径、重试按错误码、域名通配分表） |

## 4. 已关闭项（防重复劳动）

| 项 | 结论 |
|---|---|
| `keyhub` 是否可从静态取到 | **能**：`User Data/ahanet/server.json` 明文有（07 §4；先前"零命中"只是搜索漏了 `User Data/`） |
| DoubaoWork 转储的可用性 | 是**另一产品**（2.31）的内存，不作 doubao 证据；仅交叉佐证配置一致（07 §5） |
| `aha_net.dll` 是否承载握手 | **否**：strings 中无 keyhub/handshake/tt_encrypt/encrypt_config（06 §1） |
| `sbox` / `vm_sdk` / `aha_cua` | 已挖成篇 09：两层隔离（AppContainer + 受限令牌）、Host 状态机、PiP 实锤、aha_cua 模块地图 |
| `agent_infra` / `pc_push` / `deviceregister` / `DoubaoLinkRouter` | 已挖成篇 10：agent_infra 经 `aha_cua_*` C API 桥接 aha_cua；pc_push=WNS 推送；deviceregister=设备指纹；LinkRouter 独立版本 2.32.x；liblark=59 个 Lark* 类的 Feishu SDK 全量内嵌 |
| `aha_doctor` | 已挖成篇 11：硬件指纹（SMBIOS/GPU/磁盘/固件）+ 品牌/地理检测 + 崩溃上报 `pc-mon.zijieapi.com` + ETW；config/*.dat 为二进制检测数据集 |
| `ai-views` / `rpa-component-extension` | 已挖成篇 12：ai-views 在全网页注入 AI 侧边栏（DNR+cookies+bookmarks）；rpa 走 `debugger`(CDP)+`nativeMessaging` 做浏览器自动化 |
| 未深挖原生域（launcher/终端/模型/资源/pip/locales/local_webcontents） | 已挖成篇 14：启动器=**Saman Installer**；终端=**微软 ConPTY**；model/RTCSDK_RES/effects=端侧模型与效果资源；sdks 仅 aha_cua；Dictionaries 空 |
| `sscronet.dll` 导出面 | 已挖成篇 13：**1381 导出**（`FTX_`=frontierx 传输 SDK 164、`Cronet_Frontier*` 148、`BiStream` 23）；加密/帧锚点落成**可寻址 RVA** |
| sscronet TLS 层 | 标准 TLS 1.3（frida 活抓，15 §4）：`HKDF-Expand-Label`=`0x6b0a60`，RFC 8446 label；该层 PSK 为空，`bk_k` 不是它的 PSK |
| ttnet 加密通道（keyhub） | keyhub 握手：ECDH(`secp256r1`) → ChaCha20 会话加密，cipher 4097，`x-tt-e-*` 头（07 §7） |
| 单函数定位（加密/帧） | 方法为功能串 xref（`.cc` 锚点为只读数据集、0 代码引用；对照串有 xref）→ 加密握手函数群 + 密钥调度函数 + btrans 处理函数均已定位并反汇编（15 §1–§4） |