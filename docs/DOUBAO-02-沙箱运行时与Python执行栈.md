# 02 沙箱运行时与 Python 执行栈

主题：豆包"任务模式（Task Mode）"的**本地执行环境**——模型生成的代码在本机的落地形态。
证据：`app/task-mode-resource/` 目录通读 + `workspace/base-runtime/` 实测副本 + Python 包源码通读。

## 1. 沙箱运行时（`runtime-bundle/packages/base-*.zip`）

`app/task-mode-resource/runtime-bundle/` 下的 base 包解出来就是一支**沙箱化运行时**，产物已归 `workspace/base-runtime/`（实测）：

| 组件 | 版本/文件 |
|---|---|
| Python | **3.14.7**（`python.exe`、`python314.dll`、`DLLs/`、`Lib/`、`Scripts/`、`vcruntime140{,_1}.dll`） |
| Node | **v22.23.2**（`node/node.exe`、`npm.cmd`/`npm.ps1`、`npx.*`、`corepack.cmd`、`nodevars.bat`、`install_tools.bat`） |
| pip | 26.2.1，配 `pip.conf`（走字节内源/镜像） |
| ripgrep | `rg.exe` 15.2.0 |
| 激活脚本 | `activate.ps1`、`prepare.ps1`（自签） |
| 其他 | `.gnpmrc`、`LICENSE.md`、`description.md` |

意义：SKILL.md 里 "runs Python on the computer you are operating" 的"那台电脑"，指的就是这个包提供的隔离环境——**不是系统 Python/Node，而是内嵌自签运行时**（本节也是 `rea doctor` 类工具链的 Node 版本约束来源）。

## 2. Python 包层（`task-mode-resource/python-packages/`）

| 包 | 规模 | 角色 |
|---|---|---|
| `seed_computer_use` | 1 文件 | 公开壳：`from gac_runtime import computer_use as _computer_use` 全量转发 |
| `gac_runtime` | 80 文件 / 520KB | **真身**：`computer_use` / `browser_use` / `computer_use_ax` 三大模块 + wire 协议 + transport |
| `aha_browser_use` | 205KB | 浏览器操作（bu 平面）实现 |
| `seed_computer_use_ax` | — | UIA（Accessibility Tree）平面壳 |
| `seed_browser_use` | — | bu 平面壳 |

> gac = "GUI as Coding"（代码里报错前缀的原话）。协议细节见 03。

## 3. 平面划分（cu / bu / ax）

| 平面 | 模块 | 对象 |
|---|---|---|
| `cu` | `computer_use_windows` | 本机 GUI（用户会话、cookie） |
| `ax` | `computer_use_ax` | UIA 无障碍树 |
| `bu` | `browser_use` | 产品自带浏览器（干净环境） |

`COMPUTER_USE_MODULE_ID = 2`，`PROTOCOL_VERSION = 1`。**平面一经选定不得静默切换**（SKILL.md 明文要求）。

## 4. 公开 API 面（`computer_use/_windows/public/api.py`）

`screenshot` / `click` / `drag` / `scroll` / `type` / `hotkey` / `clipboard` / `list_apps` / `launch_app` / `switch_desktop` / `wait` —— 全部经 `wrap_action` 埋点（instrumentation 上报，见 03 §5）。

## 5. 技能包与 CLI（云下发侧）

`runtime-bundle/manifest.json`：`bundle_version: 20260914T100821Z`、`skill_type: 2`，含 50+ skill zip；`resources` 另有三个 CLI：`mediakit-cli`、`browser-rpa-cli`、`lark-cli 1.2.60`。技能体系细节见 04。