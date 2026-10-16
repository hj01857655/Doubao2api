# 10 宿主侧模块定档：agent_infra / pc_push / deviceregister / DoubaoLinkRouter

主题：`app/` 下其余原生模块的定性与接口面——补齐 09 之外的宿主侧拼图。
证据：`workspace/modules/` 字符串提取（`agent_infra.txt` 540KB / `pc_push.txt` 98.8KB / `deviceregister_shared.txt` 19.8KB / `DoubaoLinkRouter.exe.txt` 6,303 行 / `liblark.txt` 11.9MB）。

## 1. `agent_infra.dll`——**agent 与 CUA 的桥接层**（本篇核心发现）

| 项 | 实测 |
|---|---|
| 构建根 | `C:\103777\agent_infra\src\`（工程 id 103777） |
| 源码模块 | `shell_runtime\shell_toolchain_resolver.cpp`、`local_runtime\LocalRuntimeModule` |
| Flutter 集成 | `fml\task_runner_checker.cc`、`fml\memory\task_runner_checker.cc`（Flutter fml 框架） |
| 文件工具 | `OnRuntimeFileToolWriteAsync` / `EditAsync` / `GetInfoAsync`（nlohmann json `AsyncCommandArgs`） |
| Shell 工具链 | `shell_toolchain_resolver`——一次性进程执行 + 输出内存上限 |

### 1.1 `aha_cua` C API 绑定（关键链路）

`agent_infra.dll` 导出/调用以下 C API，**直接绑定 09 篇的 `aha_cua.dll`**：

```
aha_cua_create_tool_call_obj      # 创建工具调用对象
aha_cua_destroy_tool_call_obj     # 销毁
aha_cua_set_tool_property         # 设置工具属性
aha_cua_call_tool_async           # 异步调用工具
aha_cua_cancel_tool               # 取消
```

### 1.2 `cua:call_tool` 协议约束

```
cua:call_tool requires {method, params}
cua:call_tool params.trace_id must be a non-empty string
```

**判读**：`agent_infra` 是 Flutter/UI 层与 `aha_cua` 原生层之间的**桥接 DLL**——
UI 层（Flutter fml）经 `cua:call_tool` 协议调用 → `agent_infra` 校验 `{method, params, trace_id}` →
通过 `aha_cua_*` C API 转发给 `aha_cua.dll` 执行桌面操作。
这补齐了 02→03→09 的完整链路：

```
模型代码（沙箱内 Python）
  → GAC 协议帧（03，ChaCha20-Poly1305 + 三凭证）
    → 宿主 agent_infra（本篇，cua:call_tool + aha_cua C API）
      → aha_cua.dll（09，KeyboardInput / MouseInput / Screenshot / VisualTree / …）
        → Win32 GUI
```

## 2. `pc_push.dll`——Windows 推送通道

| 项 | 实测 |
|---|---|
| 构建根 | `D:\WinPushSDK` |
| 技术 | C++/WinRT + **WNS（Windows Notification Service）Channel** |
| 日志域 | `[lvpush]` |
| 职责 | 推送通知通道（WNS 云推送 + 本地通知） |

> "lv" = Lark/Volc 系推送 SDK；与 `liblark.dll`（166MB Lark 底座）同族。

## 3. `deviceregister_shared.dll`——设备注册

| 项 | 实测 |
|---|---|
| 接口 | `dr_init`、`dr_get_did_from_cache` |
| 字段 | `device_fingerprint_prefix`、`short_device_model` |
| 职责 | 设备指纹生成 + `did`（device id）/ `iid`（install id）注册与缓存 |

> 用于设备级身份标识与激活；与 `aha_doctor` 的环境检测互补（一个识设备，一个识环境）。

## 4. `DoubaoLinkRouter.exe`——链接路由（独立更新）

| 项 | 实测 |
|---|---|
| 位置 | `C:\Users\12925\AppData\Local\DoubaoLinkRouter\bin\{2.32.4, 2.32.5}\DoubaoLinkRouter.exe` |
| 版本 | **2.32.4 与 2.32.5 同时在装**（独立于主程序版本） |
| 附属 | `telemetry/` 目录 |
| 字符串 | 6,303 条；含 `doubao` 相关路由串 |
| 职责 | `doubao://` URL scheme 路由——外部链接唤起豆包并路由到对应页面/功能 |

> **独立版本号**说明它有独立的热更新通道，不随主程序包升级。

## 5. 其余模块速览（定性，未深挖）

| 模块 | 位置 | 定性 |
|---|---|---|
| `liblark.dll` | `app/liblark.dll`（166MB） | **Lark/Feishu SDK 全量内嵌**——见 §5.1 |
| `OpenConsole.exe` + `conpty.dll` | `app/terminal_conpty/`（1.07MB + 110KB） | ConPTY 终端——shell 工具的终端后端 |
| `aha_doctor` | `app/aha_doctor/`（252 文件 / 13MB） | 环境诊断：`aha_doctor.exe` + `task_host.exe` + `xperf.exe` + `perfctrl.dll` + `applogrs.dll` + `config/{3partyAppDetect,BrandDetect,BrowserPlugin,ModuleDetect,SoftwareDetect}.dat` + `certs/` + `resources/` |
| `launcher` | `app/launcher/` | `Doubao.exe`（1.84MB，真正的 Electron 壳启动器）+ `uninstall.exe` |
| `model/` | `app/model/` | 端侧模型：SAMI / bach / mattingmodel / skin_seg / ttfacemodel（人脸/分割/抠图） |
| `AUDIO_MODEL/` | `app/AUDIO_MODEL/`（21MB） | 音频模型资源 |
| `effects/` | `app/effects/`（158 文件） | 摄像头特效：blur_background / camera_beauty / camera_circle / camera_roundrect / roundrect_border |
| `RTCSDK_RES/` | `app/RTCSDK_RES/`（21MB） | RTC 资源（配合 `VolcEngineRTC.dll` / `RTCFFmpeg.dll`） |
| `pip/` | `app/pip/` | 仅一张 PNG（目录名误导，非 Python pip） |
| `Dictionaries/` | `app/Dictionaries/` | **空目录** |

### 5.1 `liblark.dll` 内嵌 Feishu SDK（166MB）

| 项 | 实测 |
|---|---|
| 体量 | 166MB / 11.9MB strings |
| Lark 域名 | `https://www.feishu.cn`（唯一 Lark/Feishu 域） |
| URL 总数 | 23 个唯一域名（含 `cacerts.digicert.com` 等标准 Chromium 证书 URL） |
| `Lark*` 类族 | **59 个**：`LarkAgent` `LarkApp` `LarkApplication` `LarkAusw` `LarkBio` `LarkBiz` `LarkBot` `LarkBuzz` `LarkChat` … |

**判读**：豆包内嵌了**完整的飞书 SDK**（不是轻量桥接），类族覆盖 Agent / App / Bio / Biz / Bot / Buzz / Chat 等飞书全产品线。与 `pc_push.dll`（`[lvpush]`，同 Lark/Volc 系）配套，为豆包的协作/推送能力提供底座。

## 6. 证据与产物

| 产物 | 位置 |
|---|---|
| 字符串提取（5 件） | `workspace/modules/{agent_infra,pc_push,deviceregister_shared,DoubaoLinkRouter.exe,liblark}.txt` |
| 字符串提取工具 | `workspace/tools/pystrings.py`（自包含副本） |