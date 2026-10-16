# 03 GAC 传输协议全解

主题：沙箱内的 Python 如何把"电脑操作"送回宿主执行——**本次审计最有价值的发现**。
证据：`workspace/base-runtime/` 内 `gac_runtime/transport/`、`gac_runtime/modules/computer_use_windows.py` 源码通读。

## 1. 设计要点

沙箱里的 Python **不直接摸 Win32**：所有"电脑操作"都编码成**命令帧**发给宿主，由宿主执行。这是宿主可审计/可拦截/可埋点的前提。

## 2. 通道（三选一，环境变量解析）

| 变量 | 取值 | 说明 |
|---|---|---|
| `CUA_GAC_TRANSPORT` | `tcp` | 配 `CUA_GAC_SERVER_PORT` |
| `CUA_GAC_TRANSPORT` | `named_pipe` / `uds` | 配 `CUA_GAC_ENDPOINT` |
| `CUA_GAC_FALLBACK_TRANSPORT` / `_PORT` | — | 回退通道 |

## 3. 认证三件套与帧加密

| 变量 | 作用 |
|---|---|
| `CUA_GAC_SECRET` | urlsafe_base64，**ChaCha20-Poly1305 帧加密密钥** |
| `CUA_GAC_SESSION_ID` | 会话标识 |
| `CUA_GAC_EXECUTION_ID` | 执行标识 |

- 自带**纯 Python ChaCha20/Poly1305 实现**：`chacha20poly1305.py`，含 `token_challenge` 握手。
- **每个执行会话独立** SECRET/SESSION/EXECUTION；Named Pipe 与 TCP **同一套帧协议**。
- 帧上限：响应 payload **129MB**（截图 PNG 上限 128MB）。

## 4. 命令 schema（`modules/computer_use_windows.py`）

- `COMPUTER_USE_MODULE_ID = 2`，`PROTOCOL_VERSION = 1`
- **struct 二进制帧**（用 `io` / `json` / `struct`）
- 每个 GUI 动作是一个 dataclass 命令：`WindowsComputerUseCommand.click/hover/…`
- 模块划分：`computer_use_windows`（cu 平面）、`computer_use_ax`（UIA 平面）、`browser_use`（bu 平面）

## 5. 公开 API 面与埋点

`computer_use/_windows/public/api.py`：`screenshot` / `click` / `drag` / `scroll` / `type` / `hotkey` / `clipboard` / `list_apps` / `launch_app` / `switch_desktop` / `wait`。

**动作全走 `wrap_action` 埋点**（instrumentation 上报）——即宿主对每一次 GUI 动作都有记录点。

## 6. 安全边界（协议层面的取舍）

1. **模型代码永远碰不到 Win32**——所有 GUI 动作必须经宿主中转，宿主可审计/拦截/埋点每个动作。
2. **传输加密 + 会话三凭证**——SECRET/SESSION/EXECUTION 逐会话绑定，通道与协议解耦（Pipe 与 TCP 同帧）。
3. **平面隔离强制**——`cu` 与 `bu` 严格分平面，选定后不得静默切换（SKILL.md 明文）。
4. **登录/验证码强制人工**——见 05 §4（`interaction.request_action` 的 user-takeover 门，把"需要人工"做成协议级动作而非聊天文本）。

> 可复用点（对 2api / 反代项目的启发）汇总在 99 §3。