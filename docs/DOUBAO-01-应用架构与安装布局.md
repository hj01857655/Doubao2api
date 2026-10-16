# 01 应用架构与安装布局

审计对象：`C:\Users\12925\AppData\Local\Doubao\`（豆包桌面端，Electron，Chromium **147.0.7727.149**，本机在装批次 2026-10-15）
证据方式：目录实测 + 文件大小实测 + `entry-moa/main.js` 字串提取（见 05）

## 1. 包形态：不走 asar

豆包是 Electron 但**不用 asar**，纯目录布局——所以 JS 层可直接读、可直接 grep，是本次审计成本低的主要原因。

```
Application/
├── Doubao.exe                    # 壳（272MB 进程为渲染进程，非主 exe）
└── app/                          # Chromium 147 运行时 + 全部原生 DLL + 各资源域
    ├── Doubao.dll（307.6MB）      # 主逻辑（原生，非 JS）
    ├── liblark.dll（166MB）       # 飞书/Lark 底座
    ├── sscronet.dll（9.86MB）     # ttnet 网络栈（见 06/07/08）
    ├── aha_net.dll（2.4MB）       # aha 平台网络封装
    ├── local_webcontents/（558MB）# JS 层（三域，见下）
    └── task-mode-resource/        # 任务模式本地执行栈（见 02）
User Data/                         # 运行时数据（ahanet/ 网络配置见 07）
```

> `Doubao.dll` 实测 322,524,048 字节；`sscronet.dll` 实测 9,855,888 字节。

## 2. `app/` 资源域逐项（实测）

| 目录 | 定性 |
|---|---|
| `local_webcontents/` | **JS 层**，三域：`apps`（应用包）/`biz`（业务插件）/`extensions`（扩展），见 §3 |
| `task-mode-resource/` | 任务模式（"帮我操作电脑"）本地执行栈：沙箱运行时 + Python 包 + 技能包清单，见 02/04 |
| `sbox/` | 沙箱（服务化执行环境，与任务模式的 sandbox 语义直接相关） |
| `vm_sdk/` | 虚拟化 SDK（与 PiP 虚拟桌面能力相关，待挖） |
| `launcher/` | 启动器 |
| `aha_doctor/` | 自检/诊断（aha 平台） |
| `terminal_conpty/` | ConPTY 终端组件 |
| `sdks/` | 各平台 SDK（登录/推送等，待逐项） |
| `model/`、`AUDIO_MODEL/`（21MB） | 端侧模型与音频模型资源 |
| `effects/` | 音视频特效（RTC） |
| `RTCSDK_RES/` | RTC 资源（配合 `VolcEngineRTC.dll`/`RTCFFmpeg.dll`） |
| `pip/` | 嵌入式 Python 包管理相关 |
| `locales/`、`Dictionaries/` | 本地化与词典 |

（本表目录为 `ls -d app/*/` 实测枚举；除已标注"见 xx 篇"者，其余定性为按目录名与相邻模块推断。）

## 3. `local_webcontents/` 三域

```
local_webcontents/
├── apps/       # entry-moa（main.js 2.4MB，UI 主包）、entry-main、doubao-office
├── biz/        # ahalets/ 业务插件（"aha" 为内部代号）
└── extensions/ # ai-views（AI 侧栏注入）、rpa-component-extension（浏览器自动化）
```

- `apps/entry-moa/main.js`（2.4MB）= UI 主包（moa = 豆包工作台），关键字串见 05。
- `biz/ahalets/`、`extensions/*` 的形态与职责见 05。

## 4. 与 DoubaoWork 的区分（重要，防串味）

本机另有 `C:\Users\12925\AppData\Local\DoubaoWork\`（办公版，文档见另一项目 `Agent/DoubaoWork/`）。两者是**不同产品**：

| | Doubao（本篇） | DoubaoWork |
|---|---|---|
| 本机状态 | **在装**（`Application/app/` 为 Chromium 布局） | 已不在（`Application/` 仅剩遗留脚本） |
| 版本（实测） | 2026-10-15 批次 | 2.31（仅存于旧内存转储） |
| 网络栈 | `sscronet.dll` + `aha_net.dll` | 同族 ttnet（`encrypt_config_v1` 与 doubao **逐字一致**） |

> 结论：两产品**共用同一套 ttnet 控制面配置**，但证据必须各归各——拿 DoubaoWork 的转储当 doubao 证据是错的（07 §6 记录了这个纠正）。