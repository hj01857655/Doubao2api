# 04 技能云下发体系

主题：任务模式的技能（Skill）如何被版本化下发、校验与本地落地。
证据：`app/task-mode-resource/runtime-bundle/manifest.json` + `workspace/skills/` 样本实测。

## 1. manifest 结构

| 字段 | 实测值 |
|---|---|
| `bundle_version` | `20260914T100821Z` |
| `skill_type` | `2` |
| 技能项 | **50+ 个 skill zip**，每个 = 一个 `SKILL.md` |
| 每项元数据 | `skill_id` / `version` / `checksum` |

- **与 Anthropic Agent Skills 同构**：技能即一份 `SKILL.md` 文档，云端版本化分发。
- 本地只有 **checksum 校验过的 zip**，版本与清单**全云端控制** → 可分钟级全量换血。
- 样本实测：`workspace/skills/computer-use-automation/SKILL.md`。

## 2. 技能全家桶（清单实测）

| 系 | 技能 |
|---|---|
| Lark 全家 | doc / sheet / im / mail / calendar / meeting / base / wiki / drive / task / approval / okr / attendance … |
| 豆包创作系 | creative-design / video / drama、`seedream-5.0-pro` |
| 金融系 | daily-stock / earnings-analysis / market-hotspot / industry-analysis |
| 学术系 | literature-research / evaluator / polish |
| 医疗系 | report / clinical-decision / medical-literature |
| 自动化系 | **computer-use-automation**、browser-record-replay（RPA 录制回放）、skill-creator-for-work |

## 3. 配套 CLI（`resources`）

| CLI | 用途 |
|---|---|
| `mediakit-cli` | 音视频处理 |
| `browser-rpa-cli` | 浏览器 RPA |
| `lark-cli` **1.2.60** | Lark 操作 |

## 4. 技能路由机制（行为实证）

任务模式下 agent 是**主动按需读 SKILL.md**，而非被动接收注入 prompt：

1. agent 用 `Read` 工具读工作区内的 `.skills/<name>/SKILL.md`（绝对路径）；
2. 读完按规范选择工具（如读 `computer-use-automation/SKILL.md` 后调用 `computer_use_tool`）。

> 这一点决定了"技能包换血 = 行为换血"，也决定了 SKILL.md 是审计的最短路径（规范原文即行为契约）。

## 5. 供应链控制（可复用取舍）

- `manifest + checksum + 云端版本化`，比本地目录扫描多一层供应链控制；
- 技能是**文档**，不是代码——不注入可执行逻辑，降低沙箱逃逸面；
- 配套 CLI 是本地二进制/脚本，权限面另行约束。