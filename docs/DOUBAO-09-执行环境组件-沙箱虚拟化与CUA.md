# 09 执行环境组件：沙箱、虚拟化与 CUA 原生库

主题：任务模式"操作电脑"能力的**宿主侧原生实现**——`aiep_sbox`（沙箱）/ `aiep_vm`（虚拟化·PiP）/ `aiep_ipc`（通道）/ `aha_cua.dll`（桌面自动化后端）。
证据：`app/sbox/`、`app/vm_sdk/`、`app/sdks/aha_cua/` 实测 + 字符串提取（`workspace/sbox-vm/`）。

## 1. 组件清单

| 组件 | 文件（x64 实测） | 定性 |
|---|---|---|
| 沙箱 SDK | `app/sbox/sbox_sdk.dll`（2,287,504B）、`sbox/{x64,x86}/aiep_sbox.dll`（753,552 / 572,816B） | **AppContainer 级**隔离执行 |
| 沙箱宿主 | `sbox/{x64,x86}/run_helper.exe`（682,384 / 545,680B） | 拉起目标进程的 helper |
| 共享通道 | `sbox/{x64,x86}/aiep_ipc.dll`（577,424 / 435,088B） | **两端共用**的 IPC 实现 |
| 虚拟化 SDK | `app/vm_sdk/vm_sdk.dll`（2,539,920B）、`vm_sdk/{x64,x86}/aiep_vm.dll`（908,688 / 692,112B） | 受限令牌 + **PiP 虚拟桌面** |
| CUA 后端 | `app/sdks/aha_cua/aha_cua.dll`（5,919,120B）+ `resources/` | 桌面输入/截图/可视化树/应用态 |

> `sdks/aha_cua/` **本机确实部署**（早期在另一产品的审计中曾判"optional 外挂库、本机未部署"——在 doubao 不成立）。

## 2. 构建标识（源码根，据此判定归属与版本形态）

| 组件 | 源码根 / PDB |
|---|---|
| `aiep_*`、`run_helper`、`sbox_sdk` | `C:\92068\**aha_sbox_v1**\Build\MinSizeRel\x64\pdb\*.pdb` |
| ↳ 沙箱 SDK 源码 | `C:\92068\aha_sbox_v1\src\**AIEPSBox**\SBoxSDK\Host\host.cpp` |
| ↳ 虚拟化 SDK 源码 | `C:\92068\aha_sbox_v1\src\**AIEPVM**\VMSDK\Host\host.cpp`、`...\Host\host_shared_data.cpp` |
| `aha_cua.dll` | `C:\98359\**aha_assistant**\src\aha_cua\...`（另一构建工程，id 98359） |

**结论**：沙箱与虚拟化是**同一工程（aha_sbox_v1）的两条子线**（AIEPSBox / AIEPVM），共用 `aiep_ipc` 与 `run_helper`；而 CUA 桌面自动化是独立工程（aha_assistant）。

## 3. Host 启动状态机（从日志 stage 串还原）

`sbox_sdk.dll` / `vm_sdk.dll` 的 `Host` 层日志逐阶段可读：

```
stage=CreateProcessW        # 建目标进程（CreateProcessWithInject / InjectCreatedProcess）
                            #   ↳ "[host] create process failed, code {}" / binaryType={}
   ↓
stage=inject_request        # 注入阶段 → "[host] inject dll failed, code {}"
   ↓                        #   （vm 侧另有 CreateCompatProcessWithInject 兼容路径）
stage=shared_data           # HostSharedData::Init：共享内存 + 盘符映射 + 动态配置
   ↓
stage=create_ipc            # "[host] create ipc instance failed"
   ↓
stage=ipc_init              # "[host] init ipc failed"
   ↓
WaitSubProcessInit          # 子进程初始化握手 → "wait sub process inited timeout"
```

**注入**：`CreateProcessWithInject` 在目标进程内注入 DLL（注入物未在本篇定名，列入 §8）。
**PiP 实锤**：vm 侧日志 `[host] [{}] **create PIP notepad process failed**, code {}` ——
虚拟桌面（PiP）是以 notebook 类进程为载体的独立窗口环境，与另一产品审计中记录的 "PiP 虚拟桌面初始化失败" 完全对齐。

## 4. 共享数据与文件系统重定向（`host_shared_data.cpp`）

```
[shared data] init start, size={}          HostSharedData::Init
[shared data] create share memory fail
[shared data] set drive {} mapping failed   ← 盘符映射（重定向）
[shared data] set drive mappings failed
[shared data] init dynamic config failed
[shared data] get create process event prefix failed, name {} is invalid
[shared data] share memory has inited / share data init success
```

要点：**共享内存 + 盘符映射 + 动态配置 + 创建进程事件前缀**——即子进程的
文件系统视图被重定向、宿主与子进程通过共享内存与命名事件同步。这是"沙箱里也能读写文件"的实现基础。

## 5. 两层隔离机制（实测 API 面）

| | `aiep_vm`（虚拟化） | `aiep_sbox`（沙箱，**更强**） |
|---|---|---|
| 令牌 | `CreateRestrictedToken` + `AdjustTokenPrivileges` + `CreateProcessWithTokenW` | 同上 + **`CreateProcessWithLogonW`** |
| 令牌查询 | `NtOpenProcessToken` / `NtQueryInformationToken` | 同 |
| 容器 | — | **`RtlGetAppContainerNamedObjectPath`（AppContainer）** |
| SID 构造 | — | `RtlInitializeSid` / `RtlCopySid` / `RtlLengthSid` / `RtlSubAuthoritySid` / `RtlSubAuthorityCountSid` / `RtlValidSid` / `RtlConvertSidToUnicodeString` |
| 进程创建 | `CreateProcessWithTokenW` | `NtCreateProcess` / `NtCreateProcessEx`、`CreateProcessWithTokenW` / `LogonW` |
| 通道 | `NtCreateNamedPipeFile` | 命名管道同族 |

**判读**：两条线都是"受限主体 + 命名管道"模型；`sbox` 额外启用 **AppContainer 与自建 SID**，
即隔离强度按用途分级——`cu`（操作用户本机）用 vm 路线，不可信代码执行用 sbox 路线。

## 6. 共享 IPC 层（`aiep_ipc`）

导出/符号面：

```
CreateIPCInterface / CreateIPCInterface2 / DestroyIPCInterface
InitIPCInterface2WithSecurityDescriptor     ← 带安全描述符初始化
类：IIPC（V2 实现：AVCIPCImpl / AVIIPCV）
```

即：**通道由安全描述符约束 ACL**，两侧（宿主 / 被注入进程）加载同一份 `aiep_ipc.dll`（x64/x86 各一份，与目标进程位数对齐）。

## 7. `aha_cua.dll`：模块地图与服务类

### 7.1 源码模块（`src/aha_cua/`）

| 模块 | 职责 |
|---|---|
| `desktop/input/keyboard_input_win.cpp` | 键盘输入（`get_virtual_key`，虚拟键 1..254，拒非法键） |
| `desktop/input/mouse_input_win.cpp` | 鼠标输入（`set_cursor_pos`） |
| `desktop/screenshot/win/screenshot_win.cpp` + `.../wgc/wgc_capture.cpp` | 截图（两路：常规 + **WGC**（Windows Graphics Capture）） |
| `desktop/visual_tree/visual_tree_element_win.cpp` | 可视化树（UIA） |
| `desktop/app_state/app_state_win.cpp` | 应用状态快照（`walk_tree` / `snapshot`，按 pid 缓存） |
| `desktop/app/app_win.cpp` + `installed_app_service_win.cpp` | 应用查询与启动（`run_app`）、已安装应用枚举（`load_installed_apps`、`RegisterAppsFolderWatch`、`NotificationLoop`） |
| `desktop/agent_cursor/agent_cursor_win.cpp` + `win/virtual_cursor_window.cpp` | **代理光标**与**虚拟光标窗口**（`SetCursorPath`）+ 截图合成光标 |
| `desktop/window/window_win.cpp` / `platform/platform_ui_element_win.cpp` | 窗口与平台元素 |
| `desktop/powershell/powershell.cpp` | **PowerShell 执行器** |
| `common/process.cpp` / `thread.cpp` | 进程/线程工具 |
| `signature_check_win.cpp` | **加载器签名校验**（`check_loader_signature`） |
| `tool_call_obj.cpp` / `tool_executor/state.cpp` | 工具调用对象与执行状态机（`StateToolExecutor::invoke_a11y_element`、`state.capture`） |

### 7.2 服务类（RTTI 实测）

`AgentCursorWin`、`VirtualCursorWindow`、`AppStateServiceImpl`、`AppQueryService`、`InstalledAppService`、
`KeyboardInput`、`MouseInput`、`ScreenshotService`、`WgcScreenshotService`、`VisualTreeServiceImpl`、
`VisualTreeElementImpl`、`UIAutomationContext`、`WindowImpl`、`PowerShellExecutor`、`platform`。

### 7.3 与任务模式链路的对齐

| 观察到的软件行为 | 对应原生实现 |
|---|---|
| GAC `cu` 平面（click/type/scroll/screenshot/list_apps…） | `KeyboardInput` / `MouseInput` / `ScreenshotService` / `AppQueryService` / `VisualTreeServiceImpl` |
| UIA 平面（`computer_use_ax`） | `UIAutomationContext` / `VisualTreeElementImpl` |
| 截图带"代理光标" | `AgentCursorWin::copy_screenshot_with_visual_cursor` |
| PiP 虚拟桌面失败降级为 PowerShell | `VirtualCursorWindow`（PiP 侧）↔ `PowerShellExecutor`（降级侧） |
| 技能/加载器需校验来源 | `check_loader_signature`（signature_check_win.cpp） |

## 8. 未挖（本篇边界）

- `aiep_ipc` 的**线上帧格式**（IIPC V2 方法表与序列化）——需单函数反编译（见 06 §2 工具边界）。
- 注入的 **DLL 是什么**（`CreateProcessWithInject` 的注入物）+ `binaryType` 取值表。
- 盘符映射的**具体映射表**（宿主哪个目录 → 子进程哪个盘）。
- `aha_cua/resources/` 内容。
- `aiep_sbox` 与 `aiep_vm` 的**选择策略**（谁决定用哪条线，配置在哪）。

## 9. 证据与产物

| 产物 | 位置 |
|---|---|
| 字符串提取（5 件 + SDK 层 2 件） | `workspace/sbox-vm/{aiep_sbox,aiep_ipc,aiep_vm,run_helper.exe,aha_cua,sbox_sdk,vm_sdk}.dll.txt` / `run_helper.exe.txt` |
| 字符串提取工具（自包含副本） | `workspace/tools/pystrings.py` |