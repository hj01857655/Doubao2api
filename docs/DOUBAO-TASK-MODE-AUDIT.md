# 豆包桌面端本地逆向审计（Windows，任务模式架构）

审计对象：`C:\Users\12925\AppData\Local\Doubao\Application`（Electron 桌面端，Chromium 147.0.7727.149）
审计方式：静态分析——目录结构、`task-mode-resource` 清单与 Python 包源码通读、`entry-moa/main.js`（2.4MB UI 主包）字串提取
工作区：解包产物在 `Doubao/workspace/`（base-runtime / skills 样本），本文档是结论

> **本稿已按主题拆分**：见 [DOUBAO-00-索引.md](DOUBAO-00-索引.md)——01 架构与安装布局 / 02 沙箱运行时与 Python 执行栈 / 03 GAC 传输协议 / 04 技能云下发 / 05 UI 层与扩展面；网络侧见 06–08。本文件保留为任务模式的叙事原稿。

## 包形态：不走 asar

豆包是 Electron 但**不用 asar**，纯目录布局：

```
Application/
├── Doubao.exe                  # 壳（272MB 进程的是渲染进程，不是主 exe）
├── app/                        # Chromium 147 运行时 + 全部原生 DLL
│   ├── Doubao.dll（315MB）     # 主逻辑（原生，非 JS）
│   ├── liblark.dll（166MB）    # 飞书/Lark 底座
│   └── local_webcontents/（558MB）  # JS 层（见下）
└── User Data/
```

`local_webcontents/`：
- `apps/entry-moa/main.js`（2.4MB）——UI 主包（moa = 豆包工作台）
- `apps/entry-main`、`apps/doubao-office`——入口与 Office 集成
- `biz/ahalets/`——业务插件（"aha" 是内部代号）
- `extensions/ai-views/`、`extensions/rpa-component-extension/`——Chrome 扩展形态的 AI 侧栏与 RPA 组件

## 任务模式（Task Mode）架构——本次审计核心发现

`app/task-mode-resource/` 是豆包"帮我操作电脑"能力的本地执行栈，由三部分组成：

### 1. 沙箱运行时（`runtime-bundle/packages/base-*.zip`）

- **Python 3.14.7** + pip 26.2.1 + **Node v22.23.2** + npm 10.9.8 + ripgrep 15.2.0，全部内嵌、自签 `activate.ps1` 激活
- 内置 `pip.conf`（走字节内源/镜像）
- 这就是模型代码的执行环境——`SKILL.md` 里说 "runs Python on the computer you are operating"，机器上就是这个包

### 2. Python 包层（`python-packages/`）

| 包 | 角色 |
|---|---|
| `seed_computer_use`（1 文件） | 公开壳：`from gac_runtime import computer_use as _computer_use` 全量转发 |
| `gac_runtime`（80 文件，520KB） | **真身**：computer_use / browser_use / computer_use_ax 三大模块 + wire 协议 + transport |
| `aha_browser_use`（205KB） | 浏览器操作（bu 平面）实现 |
| `seed_computer_use_ax` | UIA（Accessibility Tree）平面壳 |
| `seed_browser_use` | bu 平面壳 |

（gac = "GUI as Coding"，代码里报错前缀原话。）

### 3. Skill 包云下发体系（`runtime-bundle/manifest.json`）

- `bundle_version: 20260914T100821Z`，`skill_type: 2`
- **50+ 个 skill zip**，每个 = 一个 `SKILL.md`（与 Anthropic Agent Skills 同构），含 `skill_id`/`version`/`checksum`，云端版本化分发
- 全家桶：lark 全家（doc/sheet/im/mail/calendar/meeting/base/wiki/drive/task/approval/okr/attendance…）+ 豆包创作系（creative-design/video/drama、seedream-5.0-pro）+ 金融系（daily-stock/earnings-analysis/market-hotspot/industry-analysis）+ 学术系（literature-research/evaluator/polish）+ 医疗系（report/clinical-decision/medical-literature）+ **computer-use-automation** + browser-record-replay（RPA 录制回放）+ skill-creator-for-work
- `resources` 三个 CLI：`mediakit-cli`（音视频处理）、`browser-rpa-cli`、`lark-cli` 1.2.60

## GAC 传输协议——最有价值的发现

`gac_runtime/transport/`：沙箱里的 Python **不直接摸 Win32**，所有"电脑操作"都编码成命令帧发给宿主执行。

**通道**：三选一，从环境变量解析——
- `CUA_GAC_TRANSPORT` = `tcp`（`CUA_GAC_SERVER_PORT`）| `named_pipe` / `uds`（`CUA_GAC_ENDPOINT`）
- 回退通道：`CUA_GAC_FALLBACK_TRANSPORT/PORT`
- 认证三件套：`CUA_GAC_SECRET`（urlsafe_b64，**ChaCha20-Poly1305 帧加密**）+ `CUA_GAC_SESSION_ID` + `CUA_GAC_EXECUTION_ID`
- 自带纯 Python ChaCha20/Poly1305 实现（`chacha20poly1305.py`，含 token_challenge 握手）
- 帧上限：响应 payload 129MB（截图 PNG 上限 128MB）

**命令 schema**（`modules/computer_use_windows.py`）：
- `COMPUTER_USE_MODULE_ID = 2`，`PROTOCOL_VERSION = 1`，struct 二进制帧（`io`/`json`/`struct`）
- 每个 GUI 动作是一个 dataclass 命令（`WindowsComputerUseCommand.click/hover/…`）
- 模块划分：`computer_use_windows`（cu 平面）、`computer_use_ax`（UIA 无障碍树平面）、`browser_use`（bu 平面）

**公开 API 面**（`computer_use/_windows/public/api.py`）：screenshot / click / drag / scroll / type / hotkey / clipboard / list_apps / launch_app / switch_desktop / wait——动作全走 `wrap_action` 埋点（instrumentation 上报）。

## UI 层（entry-moa/main.js）关键字串

- `enter_computer_use` / `enter_computer_use_failed` / `computer_use_tool` / `computer_use_agent`——CUA 会话入口
- `cua_sse_sandbox_route_*`（task_state_seed / conversation_promote / profile_mismatch）——**CUA 任务状态经 SSE 通道播报**，UI 按 `sandbox_id` 分组会话、按 `profile` 匹配路由
- `sandbox_route_task_state_seed_failed`——UI 需要把任务状态"种子"进沙箱路由
- 上报域名：`mcs-nontt.byteintlapi.com`（监控）、`lf3-short.ibytedapm.com`（Slardar FE）；主业务走 `https://www.doubao.com/top/v1`
- 浏览器 RPA 动作埋点：`doubao_pc_browser_click/navigate/input/scroll`

## 安全边界设计（值得抄的取舍）

1. **模型代码永远碰不到 Win32**——所有 GUI 动作必须经宿主中转，宿主可审计/拦截/埋点每个动作
2. **传输加密 + 会话三凭证**——每个执行会话独立 SECRET/SESSION/EXECUTION，Named Pipe 与 TCP 同一套帧协议
3. **登录/验证码强制人工**——SKILL.md 明文要求调用 `interaction.request_action(type="browserControl")` 走 user-takeover 门，禁止文本请求登录或绕过
4. **skill 全程云管**——本地只有 checksum 校验的 zip，版本/清单云端控制，可分钟级全量换血
5. **浏览器双平面隔离**——`cu`（本机装的浏览器，用户会话/cookie）与 `bu`（产品自带浏览器，干净环境）严格分平面，SKILL.md 要求"选定平面后不得静默切换"

## 可复用点（对 2api / 反代项目的启发）

- **GAC 协议本身**：二进制帧 + module id + ChaCha20-Poly1305 + 三凭证会话，是"让云端模型安全操作本机"的完整参考实现，比 MCP stdio 多了加密与执行会话绑定
- **skill 下发体系**：manifest + checksum + 云端版本化，比本地目录扫描多一层供应链控制
- **`interaction.request_action` 的 user-takeover 门**：把"需要人工"做成协议级动作而非聊天文本，杜绝模型自作主张

## 未深挖（后续可选）

- `Doubao.dll`（315MB 原生主逻辑）——需 rea/IDA 级分析，纯静态字串收益低；其**网络层（sscronet 内 ttnet 私有栈）已单独成篇**：见 06 模块地图 / 07 加密握手与运行配置 / 08 body 帧格式
- `ahalets/` 业务插件（205KB browser_use 之外还有 office/worklet 等）
- `AUDIO_MODEL/`（音频模型资源，21MB）
- `pc_push.dll`（长连接推送）、`DoubaoLinkRouter.exe`（链接路由）
- 豆包与 `Copilot` 式的账户体系 / token 存储位置（在 `User Data/` 下，涉隐私未动）
