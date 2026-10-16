# 11 环境检测与诊断：aha_doctor

主题：`app/aha_doctor/`（252 文件 / 13MB）——客户端启动时对宿主环境的**检测、诊断与上报**子系统。
证据：`workspace/aha_doctor/` 字符串提取 + `aha_doctor/config/*.dat` 实测。

## 1. 组件清单

| 文件 | 职责 |
|---|---|
| `aha_doctor.exe` | 主诊断进程 |
| `task_host.exe` | 任务宿主（承载检测插件） |
| `xperf.exe` | Windows xperf 性能采集（ETW） |
| `perfctrl.dll` | ETW 追踪控制（`perfctrl.pdb`） |
| `applogrs.dll` | 应用日志 |
| `config/*.dat` | **二进制检测数据集**（5 件，见 §3） |
| `certs/` | 证书 |
| `resources/` | 资源 |

## 2. 构建标识

```
C:\0713\aha_doctor\src\src\engine\plugin_module.cc        ← 插件引擎（LoadPluginModule / UnloadPluginModule）
C:\0713\aha_doctor\src\src\doctor\brand_info_wrapper.cc   ← 品牌信息
C:\0713\aha_doctor\src\src\plugins\tools_kit\xperf.cc     ← xperf 集成
C:\0713\aha_doctor\src\third_party\chromium\base\...      ← Chromium base 库
```

工程 id **0713**，内嵌 Chromium base（containers、win\windows_version、memory\ref_counted 等）。

## 3. 检测数据集（`config/*.dat`，二进制专有格式）

| 文件 | 大小 | 检测目标 |
|---|---|---|
| `3partyAppDetect.dat` | 168,431B | 三方应用检测（已装软件扫描） |
| `BrandDetect.dat` | — | 品牌识别（OEM/厂商判定） |
| `BrowserPlugin.dat` | — | 浏览器插件检测 |
| `ModuleDetect.dat` | — | 模块检测（加载的 DLL/驱动） |
| `SoftwareDetect.dat` | — | 软件检测 |

> 格式：hex 头 `06 e0 a0 18 ...`，非 JSON/XML/protobuf 明文，为专有二进制匹配表。推测含"特征→判定"规则（文件名/注册表键/进程名 → 品牌/类型）。

## 4. 硬件与系统信息采集面（实测 strings）

| 字段 | 类别 |
|---|---|
| `smbios_version` / `board_manufacturer` / `board_version` / `board_product` | 主板 / SMBIOS |
| `microcode_version` | CPU 微码 |
| `disk_index` / `disk_connection_type` | 磁盘 |
| `dedicated_memory_bytes` / `shared_memory_bytes` / `adapter_luid` / `driver_version` / `hardware_id` / `connected_adapter_model` / `gpu_accelerated` | GPU 适配器 |
| `firmware_post_seconds` / `windows_total_boot_seconds` | 固件 / 启动时序 |

> 采集粒度到 **SMBIOS + GPU LUID + 磁盘连接类型 + 固件 POST 耗时**——足以做设备指纹与兼容性判定。

## 5. 品牌与地理检测

```
action.set.current.brandinfo       # 设置当前品牌信息
action.get.current.brandinfo       # 获取
action.get.db.brandinfo            # 从 DB 查
event.current.brandinfo.changed    # 品牌变更事件
brand_info / brand_group / geo_config
```

即：`aha_doctor` 启动时读取检测数据集 → 判定设备品牌（OEM）与地理区 → 供主程序按品牌/区差异化行为。

## 6. 上报通道

| 项 | 实测 |
|---|---|
| 监控端点 | `https://pc-mon.zijieapi.com`（pc-mon = PC monitoring） |
| 崩溃上报 | `UploadPendingCrashDumpsAndDeleteDump` |
| 事件开关 | `no-event-report`（可关闭上报） |
| 性能采集 | ETW：`EnumerateTraceGuidsEx` / `TraceSetInformation` / `EtwSetMark` / `EnableTraceEx`（perfctrl.dll） |

## 7. 与其他模块的关系

- `deviceregister_shared.dll`（10 §3）生成设备指纹 `did`/`iid`；`aha_doctor` 采集硬件细节作为指纹输入。
- 上报端点 `pc-mon.zijieapi.com` 与 07 §3.3 的 ttnet 控制面同域（`zijieapi.com`），走同一套网络栈。
- 检测结果供主程序决策：是否启用 GPU 加速、按品牌调整 UI、按地理区选 CDN 等。

## 8. 未挖

- `.dat` 数据集的**二进制格式**（特征匹配规则的结构）——需逆 `plugin_module.cc` 的加载逻辑。
- `certs/` 内容与用途（是否用于检测数据集签名校验）。
- `resources/` 内容。
- 上报数据的**字段 schema**（发往 `pc-mon.zijieapi.com` 的具体 payload）。

## 9. 证据与产物

| 产物 | 位置 |
|---|---|
| 字符串提取（4 件） | `workspace/aha_doctor/{aha_doctor,task_host,perfctrl,applogrs}.txt` |
| 检测面 grep | `workspace/aha_doctor/{grep1,grep2}.txt` |