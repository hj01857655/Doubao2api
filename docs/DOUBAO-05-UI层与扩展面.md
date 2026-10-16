# 05 UI 层与扩展面

主题：UI 主包、CUA 会话的状态通道、上报域名，以及三个扩展/插件域。
证据：`local_webcontents/apps/entry-moa/main.js`（2.4MB）字串提取 + `local_webcontents/{biz,extensions}` 目录实测。

## 1. JS 层三域

```
local_webcontents/
├── apps/       entry-moa（UI 主包 main.js 2.4MB）、entry-main（入口）、doubao-office（Office 集成）
├── biz/        ahalets/ —— 业务插件（"aha" 为内部代号）
└── extensions/ ai-views（AI 侧栏注入）、rpa-component-extension（浏览器自动化）
```

> 不走 asar（见 01 §1），所以这一层可直接读、直接 grep。

## 2. entry-moa 关键串

| 串 | 含义 |
|---|---|
| `enter_computer_use` / `enter_computer_use_failed` | CUA 会话入口与失败 |
| `computer_use_tool` / `computer_use_agent` | 工具与 agent 侧命名 |
| `cua_sse_sandbox_route_task_state_seed` / `_conversation_promote` / `_profile_mismatch` | **CUA 任务状态经 SSE 通道播报** |
| `sandbox_route_task_state_seed_failed` | UI 需把任务状态"种子"进沙箱路由 |

**语义**：UI 按 `sandbox_id` 分组会话、按 `profile` 匹配路由；沙箱路由与任务状态经 **SSE** 同步给前端。

## 3. 上报与业务域名

| 域名 | 用途 |
|---|---|
| `mcs-nontt.byteintlapi.com` | 监控上报 |
| `lf3-short.ibytedapm.com` | Slardar 前端监控 |
| `https://www.doubao.com/top/v1` | **主业务** |
| `doubao_pc_browser_click` / `_navigate` / `_input` / `_scroll` | 浏览器 RPA 动作埋点 |

## 4. 人工接管门（user-takeover）

SKILL.md 明文要求：需要登录/验证码时调用
`interaction.request_action(type="browserControl")` 走 **user-takeover 门**，
**禁止**用文本请求登录、**禁止**绕过。

意义：把"需要人工"做成**协议级动作**而非聊天文本——模型没有"求用户输密码"的自由裁量空间。

## 5. 扩展面（待逐项深入）

| 域 | 现状 |
|---|---|
| `extensions/ai-views` | AI 侧栏注入（Chrome 扩展形态） |
| `extensions/rpa-component-extension` | 浏览器自动化组件（与 04 的 browser-record-replay 技能、`browser-rpa-cli` 同族） |
| `biz/ahalets/` | 业务插件；`aha_browser_use`（205KB，03/02）之外还有 office/worklet 等 |

> 三者的内部协议（消息面、注入点、权限声明）**尚未逐项展开**，列入 99。