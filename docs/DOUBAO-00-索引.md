# Doubao 桌面端逆向文档集 · 索引

> 目标：本机在装的**豆包桌面端**（Electron，Chromium 147.0.7727.149）。
> 安装位置：`C:\Users\12925\AppData\Local\Doubao\`（`Application` = 程序本体，`User Data` = 运行时数据）。
> 工作区：`E:\VSCodeSpace\Agent\Doubao\workspace\`（base-runtime 解包产物、skills 样本、native-network 证据）。
> 除注明"推断"外，所有断言均为实测（对照本机文件验证）。

## 文档清单

| 编号 | 文档 | 内容 | 等级 |
|---|---|---|---|
| 01 | [应用架构与安装布局](DOUBAO-01-应用架构与安装布局.md) | 非 asar 布局、`app/` 资源域逐项、DLL 模块地图、local_webcontents 三域、与 DoubaoWork 的区分 | A |
| 02 | [沙箱运行时与 Python 执行栈](DOUBAO-02-沙箱运行时与Python执行栈.md) | base-runtime（Python 3.14.7 / Node 22.23.2 / pip / rg）、gac_runtime 包层、cu/ax/bu 平面、公开 API | A |
| 03 | [GAC 传输协议全解](DOUBAO-03-GAC传输协议全解.md) | 三通道解析、三凭证、ChaCha20-Poly1305 帧 + token_challenge、命令 schema、帧上限、安全边界 | A |
| 04 | [技能云下发体系](DOUBAO-04-技能云下发体系.md) | manifest/checksum/版本化、50+ 技能全家桶、resources 三 CLI、技能路由机制 | A |
| 05 | [UI 层与扩展面](DOUBAO-05-UI层与扩展面.md) | entry-moa 关键串、CUA SSE 沙箱路由、上报域名、user-takeover 门、three 扩展域 | A |
| 06 | [ttnet 私有模块地图](DOUBAO-06-ttnet私有模块地图.md) | sscronet 内 bytetransport(16)/frontierx(19)/encrypt/net_detect 模块清单 + 工具边界 | A |
| 07 | [ttnet 加密握手与运行配置](DOUBAO-07-ttnet加密握手与运行配置.md) | `encrypt_config_v1` 全量值、bk_k/bk_t(142B DER)、x-tt-e/x-tt-s 头族、30 项配置、握手入口；**§7 = keyhub 握手协议**（ECDH secp256r1 → ChaCha20） | A |
| 08 | [btrans body 帧格式](DOUBAO-08-btrans-body帧格式.md) | 8 类帧、帧头/消息字段、装配约束、30 余项 `BTRANS_*` 错误码 | A |
| 09 | [执行环境组件：沙箱、虚拟化与 CUA](DOUBAO-09-执行环境组件-沙箱虚拟化与CUA.md) | aiep_sbox/aiep_vm/aha_cua：两层隔离、Host 状态机、PiP、IPC、CUA 模块地图 | A |
| 10 | [宿主侧模块定档](DOUBAO-10-宿主侧模块定档.md) | agent_infra（aha_cua C API 桥接）/ pc_push / deviceregister / DoubaoLinkRouter / liblark(Feishu SDK) | A |
| 11 | [环境检测与诊断](DOUBAO-11-环境检测与诊断-aha_doctor.md) | aha_doctor：硬件指纹、品牌/地理检测、崩溃上报、ETW 性能采集 | A |
| 12 | [浏览器扩展面](DOUBAO-12-浏览器扩展面.md) | ai-views（侧边栏注入）/ rpa-component-extension（CDP 自动化 + nativeMessaging） | A |
| 13 | [sscronet 导出面与传输 API 地图](DOUBAO-13-sscronet导出面与传输API地图.md) | 1381 导出家族、FTX=frontierx 传输 SDK、Cronet_Frontier、ttnet 私有参数面、加密/帧锚点 RVA、新工具链 | A |
| 14 | [剩余原生域补完](DOUBAO-14-剩余原生域补完.md) | 启动器(Saman Installer 2.32.5)/终端(微软 ConPTY)/端侧模型(model·RTCSDK_RES·effects)/pip/local_webcontents/helper exe | A |
| 15 | [ttnet 加密帧层再深挖与函数定位尝试](DOUBAO-15-ttnet加密帧层再深挖与函数定位尝试.md) | 加密/帧函数已定位+反汇编（功能串 xref 法）、函数级密钥调度（frida 活抓）、TLS 层 vs 加密通道分层、btrans 帧结构字段 | A |
| 99 | [未深挖与后续](DOUBAO-99-未深挖与后续.md) | 剩余硬骨头、未深挖原生域清单、可复用点、已关闭项 | — |

## 一句话结论

豆包桌面端 = **Electron（非 asar）壳 + Chromium 147 定制运行时 + 原生业务层（Doubao.dll / liblark.dll）**；
"帮我操作电脑"能力由 `task-mode-resource/` 提供**自签沙箱运行时**（Python/Node 内嵌），
通过 **GAC 协议**（二进制帧 + ChaCha20-Poly1305 + 三凭证会话）把模型生成的 GUI 动作送回宿主执行——
**模型代码永远碰不到 Win32**；技能与工具面全部**云端版本化下发**。
网络侧走字节自研 **ttnet**（cronet fork），加密通道（keyhub 握手 + bk_k/bk_t 基钥票据 + btrans body 帧）
的**全部配置面本机明文可取**（`User Data/ahanet/server.json`）。

## 文档关系

| 文件 | 关系 |
|---|---|
| `DOUBAO-TASK-MODE-AUDIT.md` | 本集的前身（任务模式审计原稿），保留完整；其内容已按主题拆入 01–05 |
| `DOUBAO-TTNET-ENCRYPT-STACK.md` | 本集的 ttnet 前身稿，内容已拆入 06–08（该文件现为索引存根） |

## 方法论警示

1. **不走 asar** → JS 层可直接 grep；minified JS 单行超长，检索一律用 `grep -aoE`，勿整文件读入。
2. **别把两个产品混起来**：本机另有 `Local\DoubaoWork\`（办公版 2.31，已不在），那是**另一个项目**的证据来源（见 01 §4、07 §5）。
3. **证据优先级**：客户端自身落盘配置（`User Data/ahanet/server.json`）> 二进制 strings > 内存转储；
   内存转储必须先用产品标识/版本串**验明宿主进程**再用。
4. **大二进制先探体量**：~10MB 级（sscronet 9.86MB）时 `rea analyze` 会 330s 超时（06 §2），直接走 strings。