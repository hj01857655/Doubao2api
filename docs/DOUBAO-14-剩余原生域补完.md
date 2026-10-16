# 14 剩余原生域补完：启动器 / 终端 / 端侧模型与资源域

主题：收口 [99 §2](DOUBAO-99-未深挖与后续.md) 列出的"未深挖原生域"逐项。
证据：直接读取安装目录 `app/` 下对应文件（strings + 文件名 + 落盘 JSON），非推断的均标注实测。

## 1. 启动器 `app/launcher/`

`launcher/` 含 `Doubao.exe`（1.84MB 壳）+ `uninstall.exe`，实为字节的 **Saman Installer** 框架（strings 内 i18n 键 `Saman_Installer_*`）。

| 项 | 值（实测） |
|---|---|
| `product_version` | **2.32.5** |
| `chromium_version` | 147.0.7727.149 |
| `exe_file_name` / `browser_file_name` | `Doubao.exe` / `Doubao_browser_proxy.exe` |
| `install_dir_name` / `data_dir_name` / `old_data_dir_name` | `Doubao` |
| `product_app_id` | `582478` |
| `product_info_key` / `autolaunch_key` / `uninstall_key` | `Doubao` |
| `homepage` | `https://www.feishu.cn/` |
| 提权 | `asInvoker` / `uiAccess=false` |

i18n 消息：`Installer version is too low` / `System version is too low` / `HasNewerVersionInstalled` / `FetchVersionFail` / 安装目录删除失败回退清注册表。
**内嵌 `-----BEGIN RSA PUBLIC KEY-----`** —— 更新包签名校验公钥（Saman 安装器的完整性校验）。
版本 2.32.5 与 [10 §5](DOUBAO-10-宿主侧模块定档.md) 的 `DoubaoLinkRouter` 2.32.x 一致。

## 2. 终端 `app/terminal_conpty/`

`conpty.dll`（109,920 字节）+ `OpenConsole.exe` —— **微软 ConPTY**（Microsoft 签名 / 时间戳链 Microsoft Code Signing PCA 2024）。

| 导出（ConPTY 标准） | 增量（OEM） |
|---|---|
| `ConptyCreatePseudoConsole` | `ConptyReparentPseudoConsole` |
| `ClosePseudoConsole` | `ConptyShowHidePseudoConsole` |
| `ResizePseudoConsole` | `ConptyClearPseudoConsole` |

构建路径 `C:\__w\1\s\bin\x64\Release\conpty.pdb`。**定性**：豆包终端能力直接复用系统级 ConPTY，非自研 PTY 层。

## 3. 端侧 AI 模型 `app/model/`

| 路径 | 命名 | 定性 |
|---|---|---|
| `model/mattingmodel/tt_matting_v15.0.model` | tt_matting | 人像抠像 |
| `model/skin_seg/tt_skin_seg_v5.0.model` | tt_skin_seg | 肤色/皮肤分割 |
| `model/ttfacemodel/tt_face_v11.0.model` + `tt_face_extra_v14.0.model` | tt_face | 人脸检测 + 关键点/属性 |
| `model/SAMI/aec44k_v1.2.model` | aec44k | 44kHz 回声消除（AEC） |
| `model/bach/tt_c1_small_v8.0.model` | tt_c1_small | 小型端侧模型（配 `algorithmConfig.json`，用途按命名保留） |

每个模型目录带 `algorithmConfig.json`（图式管线：`nodes[]` = `imageProducer` / `scene_recognition` / …，`mode` / `version`）。

## 4. 音视频资源 `RTCSDK_RES/` 与 `effects/`

- **`RTCSDK_RES/models/`** —— VolcEngine RTC 音频处理模型族：
  AI 降噪 `common_ainr`、`fullbandnr`、`newreverbnr`、`reverbnr`、`howlnr`、`crn`；去回声 `deresecho`；`pdns_*`（freq/global_emb/local_emb/se/time——个人 DNS/降噪子模型）；`quality_v1`、`rooms_v2`/`rooms_v3`（房间音效）。
  配合 `VolcEngineRTC.dll` / `RTCFFmpeg.dll` / `audioeffect.dll` / `avecodec-*.dll`。
- **`effects/`** —— 相机/背景虚化 **shader 效果包**：`blur_background`、`camera_beauty`、`camera_circle`、`camera_roundrect`、`roundrect_border`。
  内含 `AmazingFeature/`：`xshader/*.frag|*.vert`、`material/*.material`、`mesh/*.mesh`、`rt/*.rt`（渲染目标）、`sticker.config`、`texture/*.texture`；
  `algorithmConfig.json` 同样是图式管线（`ext_texture_producer` → `texture_blit` → …）。

## 5. 包管理 `app/pip/`

`pip/` 目录本身仅一个图标（`cursor-blue-white.png`）；实体是根目录的 **`pip_main.exe`** + `pip_sdk.dll` / `pip_remote.dll`。
**定性**：嵌入式 Python 包管理入口（对接 [02](DOUBAO-02-沙箱运行时与Python执行栈.md) 的 base-runtime Python）。

## 6. `local_webcontents/` 版本面与结构

```
local_webcontents/
  apps/    entry-main/ (main.js,manifest.json) · entry-moa/ · doubao-office/ · env.json
  biz/     ahalets/ · biz.pak · env.json
  extensions/  ai-views/ · rpa-component-extension/
```

`env.json` 为 **SCM 版本化**标记（对齐 [04 技能云](DOUBAO-04-技能云下发体系.md) 的版本化下发）：

| 文件 | ScmProductionVersion | ScmRepoId | commit |
|---|---|---|---|
| `apps/env.json` | 1.0.4.3479 | 576584 | 9c062ca0… |
| `biz/env.json` | 1.0.6.6031 | 500657 | 9c062ca0… |

（`entry-moa` 的 UI 层分析见 [05](DOUBAO-05-UI层与扩展面.md)；`extensions/` 两个扩展见 [12](DOUBAO-12-浏览器扩展面.md)。）

## 7. `app/` 根 helper 可执行文件清单

| exe | 定性 |
|---|---|
| `Doubao.exe` | 主程序壳（对齐 10 §5） |
| `DoubaoLinkRouter.exe` | 链接路由（独立版本，10 §5） |
| `Doubao_browser_proxy.exe` / `Doubao_proxy.exe` | 浏览器/网络代理 |
| `file_search_host.exe` / `file_utils_host.exe` | 文件检索/文件工具宿主进程（沙箱外宿主侧） |
| `ghelper.exe` | 辅助进程 |
| `pip_main.exe` | 嵌入式 Python 包管理（§5） |
| `push_detect.exe` | 推送检测（pc_push 体系，10 §3） |
| `repair.exe` | 修复工具 |
| `ttdaemon.exe` | ttnet 守护 |
| `update.exe` | 更新器（Saman，§1） |

## 8. 本次未动 / 无逆向价值

- `Dictionaries/`：**空目录**。
- `locales/*.pak`：Chromium 语言包（资源，无逆向价值）。
- 账户 / token 存储（`User Data/` 下）：**涉隐私，仍未动**（仅取过 `ahanet/` 网络配置，见 07）。

## 一句话结论

`99 §2` 的"未深挖原生域"已逐项收口：启动器 = 字节 **Saman Installer**（2.32.5 + 内嵌 RSA 更新签名）；
终端 = **微软 ConPTY** 复用；`model/` + `RTCSDK_RES/` + `effects/` = **端侧 AI/音频/shader 模型与效果资源**（命名可辨）；
pip / local_webcontents / helper exe 各自定性完成。**无新增自研协议层**——原生域的价值到此基本探尽。