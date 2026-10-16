# 01 `/samantha/chat/completion` 契约还原（Phase 1 起点）

> ⚠️ **本文已被实测部分推翻，保留作 `/samantha/*` 那条协议的参考。**
> 实测登录态抓包见 [03 桌面真实样本](doubao2api-03-desktop-chat-completion.md)（响应侧细节见 [04](doubao2api-04-sse-response-deepdive.md)）：
> 真实走的是 **`/chat/completion`**（体为 `client_meta/messages/option/user_context/ext` 五段式，
> 事件为 `SSE_ACK`/`STREAM_CHUNK`/`CHUNK_DELTA`/`SSE_REPLY_END` 字符串名），
> 不是本文推演的 `/samantha/chat/completion` + `completion_option` + 数字事件枚举。
> 两套协议在前端并存（`cfz` = SamanthaChatProtocolPlugin 走 `/samantha/*`）。

> 来源：**前端 bundle 静态还原**（非抓包推断），一切结论都标了模块名可回溯。
> 关键 chunk：`gwu.js`(模块 `cfz` = SamanthaChatProtocolPlugin) / `ly.js`(模块 `kXo`,`C7c`) /
> `s2-submit-node-generate-message.js`(模块 `IiA`) / `s2-chat-runtime.js`(模块 `Tir` 是旧路径)。
>
> **本文是静态推演，未经真实请求验证。** 未取到的东西见 §6，全部要靠一次登录态抓包关闭。

## 0. bundle 怎么再取（`tools/bundle/` 不入库，2.9 MB）

CDN 上的静态资源，无凭证、无需登录：

```powershell
$base = 'https://lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/chat/static/js/async/'
# 1) 取页面，解析两张表：id->name（238 条）与 id->contenthash（1997 条）
$page = (Invoke-WebRequest 'https://www.doubao.com/chat/' -UseBasicParsing).Content
#    l.u(e) = "static/js/async/" + (nameMap[e]||e) + "." + hashMap[e] + ".js"
# 2) 文件名 = <name>.<hash>.js（nameMap 没有该 id 时用 id 本身）
```

入口三件套在 `static/js/`（非 async）：`chat.<hash>.js`、`zr.<hash>.js`、`lib-polyfill.<hash>.js`。
**hash 会变**，每次重取都要从 `www.doubao.com/chat/` 现算，别用旧 hash。
（实测 2026-10-16：入口从 `chat.bb8ef01bf6` 变为 `chat.69ebe0327a`，本地 `tools/bundle/` 那份已失效；
且入口 JS 实际在 CDN `https://lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/chat/static/js/<name>.<hash>.js`
——`www.doubao.com/static/js/...` 会返回 HTML 兜底页。）

**chunk 名/hash 映射表**就内联在 `https://www.doubao.com/chat/` 的 HTML 里（`l.u=function(e){return"static/js/async/"+({…nameMap…})[e]+"."+{…hashMap…}[e]+".js"}`，
nameMap 793 条）。按 id 拼异步 chunk URL 的现成工具：`tools/chunkurl.py <id>`（在 `tools/bundle/` 下运行，
读该目录的 `_page.html`）→
`https://lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/chat/static/js/async/<name>.<hash>.js`。


## 1. 端点与传输

| 项 | 值 | 证据 |
|---|---|---|
| URL | `POST https://www.doubao.com/samantha/chat/completion` | `gwu.js` 模块 `cfz._sendMessage` |
| 响应 | `Content-Type: text/event-stream`（SSE） | `gwu.js` 模块 `x1l`：`a:"text/event-stream"`, `b:"last-event-id"` |
| 鉴权 | `credentials:"same-origin"` + `Cookie` | `cfz._sendMessage` 调用参数 |
| 请求头 | `content-type: application/json`、`Agw-Js-Conv: str`、通用头来自 `getCommonHeaders()` | 同上 |
| 压缩 | `tt_compress` 配置只对 `/chat/completion` 前缀开启（07 §2） | `DOUBAO-07` |

另有 **第二入口** `/samantha/chat/async/stream`（`gwu.js`），旧版 `stream_call_bot` 路径在
`s2-chat-runtime.js` 模块 `Tir` 里仍可达（`/samantha/chat/completion` 同 URL，但事件常量不同）。
`s2-chat-runtime-sse-request-context-service`（chunk 382）只提供 `buildSSERequestMeta` /
`createSSERequestContextService`，负责把 `getCommonParams/getCommonHeaders` 注入每次请求。

## 2. 请求体（`_sendMessage` → `body`）

```js
body: JSON.stringify({
  ...sentEvent,
  completion_option: { ...sentEvent.completion_option, event_id: lastEventId }  // lastEventId 默认 "0"
})
```

`sentEvent` 由 `kXo`(chunk `ly.js`) 的 `u()` 生成，字段如下。

### 2.1 顶层

| 字段 | 来源 | 说明 |
|---|---|---|
| `messages` | `eb.filter(c.h)` | 每条消息经插件 `messageToSamanthaChatMessage` 转换后的数组 |
| `completion_option` | 见 2.2 | 大量开关，多数来自 `message[0].ext` |
| `evaluate_option` | `{web_ab_params, penetrate_ext}` | `web_ab_params` 来自 `ext.web_ab_params`；`penetrate_ext` = `resourcemention:"1"` 时出现 |
| `related_share_id` | `ext.related_share_id` | |
| `section_id` | `e[0].section_id` | |
| `conversation_id` | `e[0].conversation_id` | **首轮为 undefined（需要新建会话）** |
| `local_conversation_id` | `e[0].local_conversation_id` | 客户端生成的本地会话 ID |
| `local_message_id` | `e[0].local_message_id` | 客户端生成的本地消息 ID |
| `message_id` | `e[0].message_id` | 重生成/续写时才有 |
| `reply_id` | `e[0].reply_id` | |
| `thread_source` | `ext.thread_source` | 出现才带 |

### 2.2 `completion_option`

| 字段 | 来源 |
|---|---|
| `is_regen` | `ext.regen === "1"` |
| `with_suggest` | `ext.answer_with_suggest === "1"` |
| `need_create_conversation` | `!!e[0].local_conversation_id` |
| `launch_stage` | `sendMessage(e, {launchStage})` 传入 |
| `is_replace` / `is_delete` | `ext.replace` / `ext.delete === "1"` |
| `is_ai_playground` | `ext.is_playground === "1"` |
| `is_old_user` | `ext.is_old_user === "1"` |
| `user_ug_source_info` | `ext.user_ug_source_info` |
| `memory_type` | `sendMessage` 第二参 |
| `message_from` | `ext.send_from_*` 五选一枚举：`SharedApp` / `DesktopLauncher` / `DesktopPush` / `GuidanceOrSuggestion` / `CanvasActionbar`，默认 `InputBox` |
| `onboarding` | `ext.samantha_onboarding` |
| `artifact_key` | `ext.artifact_key` 或 `ext.input_skill.variables.artifact_key` |
| `action_bar_skill_id` | `e[0].skill.skill_type` |
| `disable_canvas` | `ext.input_skill.variables.disable_canvas` |
| `use_deep_think` | `ext.use_deep_think === "1"` 或 `"2"`(→`use_auto_cot`) |
| `selected_regen_id` | `ext.lastRegenerateSelectedMessageId` |
| `shared_app_name` / `shared_app_id` | `ext.send_from_desktop_share_app === "1"` 时 |
| `resend_for_regen` | `ext.resend_for_regen === "1"` |
| `enable_commerce_credit` | `ext.commerce_credit_config_enable === "1"` |

### 2.3 `messages[]` 元素

**双向插件机制**（实测；实现 chunk = `async-skill-message-protocol-transform.js`，映射表 id `74`）：

- **发送方向（内部消息 → `messages[]` 元素）**= `plugin.messageToSamanthaChatMessage(msg)`，
  匹配器 `matchPluginForMessageToAliceChatMessage`，插件由 `pluginIdentifier` 定位。
- **接收方向（Samantha 消息 → 内部）**= `plugin.aliceChatMessageToMessage(msg, ctx)`，
  匹配器 `matchPluginForAliceChatMessageToMessage`；无匹配回退 `(msg,"content","content_block")`。

**`messages[]` 元素形态**（各技能插件一致）：`{ content, content_type, attachments, references }`，
其中 `content` = `(0,UbS.b)({text, ...})`（文本块序列化），`content_type` 取自枚举
`SG.b[NR.c.<Skill>]`（`SkillDeepResearch` / `DesExtSkillReferenceAsk` / `SkillSearchEngine` / `SkillSearch` / …）。
逐插件实现见抓下的 `_ck_74.js`。从 `IiA` 可见原始消息形态：

```
{
  content_blocks_v2: [...],     // 文本块来自 mapTextToTextBlock(text)
  unique_id,                    // uuid
  message_status, message_status_v2,
  ext: {
    chat_ability: JSON.stringify(abilityParams),
    user_context, conversation_mode, use_submit_pipeline: "1",
    reference_content,          // refContents 的 JSON 串
    ...
  }
}
```

文本块结构：`{ block_type, content, block_id, parent_id, meta_info, extra_info, append_fields }`
（`IiA` 的 asr 块示例：`block_type: BLOCK_LOADING`，`content:{loading_block:{text:"asr:<id>",scene:0}, pc_event_block:""}`）。

## 3. SSE 事件协议

事件类型枚举（`s2-im-sdk-message-service.js` 模块 `fyf`）：

| 值 | 名 | 含义 |
|---|---|---|
| 0 | Unknown | |
| 1 | HEARTBEAT | 心跳 |
| 2001 | CMPL | 一条消息增量（`Tir` 里 `case CMPL → onMessage`） |
| 2002 | ACK | 确认 |
| 2003 | FIN | 流结束（`onFinish(true)`） |
| 2004 | CMD | 命令（默认忽略） |
| 2005 | ERR | 错误，`event_data` 里有 `message` |
| 2006 | VUI_CMPL | |
| 2007 | OAPI_CMPL | |
| 2010 | VERBOSE | |
| 2011 | VERSION | |
| 2022 | BLOCKLIST | |

SSE 帧字段：`{ data, event, id, retry, signature }`，`data` 内是
`{ event_type, event_data, event_id }`（`gwu.js` `cfz` onMessage 里 `(0,M.a)(l,{})` 解出）。
`event_id` 会回填到下次请求的 `completion_option.event_id`（断线重连/续传）。

CMPL 的 `event_data` 归一化结果（`gwu.js` 模块 `hG9`）：

```
{
  bot_id, conversation_type, message_type: 2001,
  conversation_id, local_conversation_id, message_id,
  index: message_index, is_delta, has_suggest, reply_id, status,
  user_type: "AIBOT", chunk_seq: event_id, local_info,
  content_obj: message.content, section_id, is_regen, regen_active,
  ext: { is_finish: "0" | "1" },
  stream_id, tts_content, message_action_bar: { invisible, config },
  ...input_content_type 相关字段
}
```

`ext.is_finish === "1"` 表示该消息生成完毕（`hG9` 里 `A = e.is_finish ? "Success" : undefined`）。

## 4. 客户端 fetch 流实现（复现要点）

`gwu.js` 模块 `Lqa` 是一个自带 SSE parser 的 fetch 封装，行为：

1. `fetch(input, { ...requestOptions, headers, signal })`，`headers` 为二维数组或 `Headers`
2. 注入 `last-event-id` 头（`x1l.b`）
3. `await openInterceptor(response)` → 校验 `response.ok`、`response.body`、
   **`content-type` 必须以 `text/event-stream` 开头**，否则 `await response.json()` 取 `{msg, code}` 抛错
4. `response.body.getReader()` 循环读，喂给 `class u`（SSE 行解析器）
5. 解析器产出 `{data, event, id, retry, signature}`，逐条 `emit("message")`

错误响应（非 SSE）走同一个 openInterceptor 分支，body 是 `{code, msg}` JSON ——
**这正好是反代要转成 OpenAI 错误格式的地方**。

## 5. 已验证的最小链路（匿名态）

`ttwid` cookie（访问 `https://www.doubao.com/chat/` 自动获得，127 字符）+ 以下请求头即可：

```
POST /alice/user/get_web_anon_id
content-type: application/json
agw-js-conv: str
origin: https://www.doubao.com
referer: https://www.doubao.com/chat/
accept: application/json, text/plain, */*
cookie: ttwid=...
```

返回 `{"code":0,"uid":"","web_id":"7697215747172812303"}`（有 `ttwid` 时 `web_id` 非 0；
无 cookie 时 `uid`/`web_id` 全 0）。说明**匿名设备态可建立**，`uid` 需要登录。

## 6. 剩余缺口

| # | 缺口 | 下一步 |
|---|---|---|
| 1 | `messages[]` 元素字段 —— **已定位**：chunk `async-skill-message-protocol-transform.js`（映射表 id `74`，本文档目录下 `_ck_74.js`）含整族转换插件；元素形态 = `{content, content_type, attachments, references}`，`content_type` 为技能枚举 `SG.b[NR.c.*]` | 逐技能枚举补齐，或用真实抓包核对 |
| 2 | `getCommonHeaders()` / `getCommonParams()` 的**真值** —— **已关（实测）**：169 条 `www.doubao.com` POST 的请求头名全集只有标准浏览器头 + `agw-js-conv` + `cookie`，**无任何 `x-tt-*` 签名头**；common params 由模块 `ybN` 构造（`aid`/`real_aid`/`device_platform`/`doubao_device_platform`/`version_code`/`pc_version`/`doubao_pc_version`/`language`/`region`/`sys_region`/`pkg_type`/`device_id`/`web_id`/`tea_uuid`/`samantha_web:1`/`web_platform`/`use-olympus-account:1`），实测缺省也能通 | 登录态下再核一遍真值即可 |
| 3 | 首轮 `conversation_id` 为空时，服务端是自动建会话还是要求先调 `/alice/conversation/create` | 抓真实请求 |
| 4 | `/samantha/chat/async/stream` 与 `/samantha/chat/completion` 的分工 | 对比两次真实请求 |
| 5 | `webmssdk` 加签 —— **部分关闭**：**HTTP 请求头侧确认无签名**（169 条 POST 头名全集只有标准浏览器头 + `agw-js-conv` + `cookie`，真实登录态请求同样如此）；`a_bogus`/`msToken` 在 **query** 上，`/alice/user/launch` 带假 `a_bogus` 仍 `code=0`（该端点不校验），但**模型端点 `/chat/completion` 上尚未证伪**——匿名请求先被 `Invalid User ID` 拦下，无法判定签名是否检查。`msToken` 由服务端经响应头 `x-ms-token` 轮转签发 | **未决风险**，带登录态省略 `a_bogus`/`msToken` 复测（见 03 §7.1）；详见 [02](doubao2api-02-gateway-no-signature.md) |

## 7. 抓包前置（Phase 0 已完成的部分）

已有样本：`tools/samples/manifest.jsonl` 现有 516 条带请求头的记录（30+ 个 host），
仍**没有一条模型请求**（`/samantha/chat/completion` 命中 0）。缺的是登录态下的一次真实发消息。
签名关卡已被实测证伪（见 §6 #5 与 [02](doubao2api-02-gateway-no-signature.md)），
所以**唯一阻塞 = 登录态 cookie**。
