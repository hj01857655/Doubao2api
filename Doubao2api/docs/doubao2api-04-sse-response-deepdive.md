# 04 `/chat/completion` SSE 响应侧深挖

> 定位：[03 桌面真实样本](doubao2api-03-desktop-chat-completion.md) 是**权威定案**（端点/query/头/体/cookie/事件全覆盖）。
> 本文只补响应侧的实现级细节：事件时序、`patch_object` 补丁语义、`end_type` 三段收尾、
> 超时参数来源、`model_item_key` ↔ `model_type` 的映射关系。冲突处以 03 为准。
> 原始样本为 2026-10-16 22:35 一次真实对话（"你好" → "你好呀！有什么我可以帮你的吗？"）。
> 凭证脱敏：Cookie / `msToken` / `a_bogus` / `x-ms-token` / `sec_sender` / 签名 URL 一律只记**名与长度**，值不落盘。
> 请求侧（端点、query、头、五段式体）见 03 §1–§5，本文 §1–§3 仅保留与响应解析直接相关的摘要。

## 0. 这条请求的身份：桌面客户端，不是 Web

| 判据 | 值 |
|---|---|
| UA | `… Chrome/147.0.0.0 Safari/537.36 **SamanthaDoubao/2.32.5**` |
| `client_platform` / `doubao_device_platform` / `web_platform` | `pc_client` / `desktop` / `desktop` |
| `runtime` / `runtime_version` / `chromium_version` | `web` / `3.41.2` / `147.0.7727.149` |
| `aid` / `real_aid` | **582478**（桌面；Web 侧是另一个 aid） |
| `sec-fetch-site` | `cross-site`（Electron 内页面源与 www.doubao.com 不同源） |
| `referer` | `https://www.doubao.com/` |

**关键**：这条走的是桌面 `sscronet`/ttnet 原生栈，却以**明文 JSON + 标准 TLS** 被抓到
→ 印证 README「桌面端 ttnet 吃命令行代理」「业务帧本身是明文」两条判读，
`x-tt-e-*`/`x-tt-s-*` 加密头族**没有出现在这条请求里**（响应只有一个 `x-tt-e-b: 1` 标记）。

## 1. 请求行

```
POST https://www.doubao.com/chat/completion?aid=582478&channel=win&chromium_version=147.0.7727.149
  &client_platform=pc_client&device_id=<REDACTED_LEN15>&device_platform=web
  &doubao_device_platform=desktop&doubao_pc_version=2.32.5&fp=verify_<device_id>
  &language=zh&pc_version=2.32.5&pkg_type=release_version&real_aid=582478&region=CN
  &runtime=web&runtime_version=3.41.2&samantha_web=1&sys_region=CN&tea_uuid=<device_id>
  &tz_name=Asia%2FShanghai&use-olympus-account=1&version_code=20800&web_id=<REDACTED_LEN19>
  &web_platform=desktop&web_tab_id=<uuid>&msToken=<REDACTED_LEN~148>&a_bogus=<REDACTED_LEN~156>
```

- 路径是 **`/chat/completion`**，不是 `/samantha/chat/completion`。
  bundle 里 `/chat/completion` 出现在 webmssdk 的 `paths.include`（`chat.bb8ef01bf6.js` 模块 `of`）与
  `tt_compress.prefix_path`（DOUBAO-07 §2）；`/samantha/chat/completion` 是 `cfz`
  (`SamanthaChatProtocolPlugin`) 的**另一套较新协议**。两套并存，实测走的是前者。
- `fp` = `verify_` + `device_id`；`tea_uuid` = `device_id`；`s_v_web_id` cookie 亦为 `verify_…` 形态。
- `web_tab_id` 是每标签页一个 UUID。
- `msToken` + `a_bogus` 由 webmssdk 注入（见 [02](doubao2api-02-gateway-no-signature.md)：网关对二者**不校验**，待在本端点复测）。

## 2. 请求头（脱敏清单）

| 头 | 值 / 说明 |
|---|---|
| `content-type` | `application/json` |
| `agw-js-conv` | `str, str`（**值重复两次**，疑似客户端拼接缺陷；反代按单值 `str` 发即可，需实测） |
| `last-event-id` | `undefined`（首包，JS 侧 `String(undefined)` 未初始化。续传时才是数字） |
| `x-flow-trace` | `04-<32hex>-<16hex>-01`（traceparent 形态，非鉴权） |
| `accept` | `*/*` |
| `origin` / `referer` | `https://www.doubao.com`（请求行里无 origin，`referer` 有） |
| `priority` | `u=1, i` |
| `sec-ch-ua*` / `sec-fetch-*` / `accept-encoding` / `accept-language` | 标准浏览器头 |
| `cookie` | 30 个 cookie 名，见 §2.1 |

### 2.1 Cookie 名清单（值一律不落盘）

会话/身份：`sessionid` `sessionid_ss` `sid_tt` `sid_guard` `sid_ucp_v1` `ssid_ucp_v1` `multi_sids`
`uid_tt` `uid_tt_ss` `n_mh` `d_ticket` `bd_sso_hi3jfd` `passport_auth_status` `passport_auth_status_ss`
`passport_mfa_token` `session_tlb_tag` `passport_csrf_token` `passport_csrf_token_default`
设备/风控：`ttwid` `odin_tt` `s_v_web_id` `biz_trace_id`
业务：`LARK_SUITE_ACCESS_TOKEN`（ES256 JWT，`access_mode:delegated`，含 `doubao_uid`/`device_id`/`cnf`）
`feishu_dpop_keypair` `flow_account_sync_event` `is_staff_user` `has_biz_token` `i18next` `dbx-web-theme`

**鉴权真正需要的最小集**（待 §7 实验确认）：`sessionid`/`sid_tt`/`uid_tt`/`d_ticket`/`ttwid`/`s_v_web_id`
+ `passport_csrf_token`（此端点未见 CSRF 头，`x-tt-agw-login: 1` 说明网关按 cookie 判登录态）。

## 3. 请求体（完整字段，实测 `content-length: 2495`）

**与 docs/01 §2 的 `kXo` 推演不同** —— 那是 `/samantha/chat/completion` 的形态。本端点是
`{client_meta, messages, option, user_context, ext}` 五段式：

```jsonc
{
  "client_meta": {
    "local_conversation_id": "local_2072112497206180",   // 客户端生成，local_ 前缀
    "conversation_id": "",                                // 首轮留空 → 服务端建
    "bot_id": "7338286299411103781",                      // 豆包主 bot
    "last_section_id": "",
    "last_message_index": null,
    "local_permissions": [ {"permission_name":"ACCESS_COARSE_LOCATION","status":1}, … ]  // 3 项定位权限
  },
  "messages": [{
    "local_message_id": "bef6d8f0-c96e-11f1-8ec4-9b95797a1ee2",   // UUID v1（客户端时钟序列）
    "content_block": [{                                            // 注意：单数 content_block
      "block_type": 10000,                                         // 10000 = 文本块
      "content": { "text_block": {"text":"你好","icon_url":"","icon_url_dark":"","summary":""},
                   "pc_event_block": "" },
      "block_id": "b2cf1343-2cda-4409-bede-87a8ead97231",          // 客户端生成 UUID
      "parent_id": "", "meta_info": [], "extra_info": {}, "append_fields": []
    }],
    "message_status": 0
  }],
  "option": { … 见 3.1 … },
  "user_context": [],
  "ext": {
    "agent_mode": "2", "use_deep_think": "0", "sub_conv_firstmet_type": "1",
    "general_task_param": "{\"server_do_compare\":true}",           // 字符串化 JSON
    "collection_id": "", "is_finish": "1",
    "conversation_init_option": "{\"need_ack_conversation\":true}", // 字符串化 JSON
    "commerce_credit_config_enable": "0"
  }
}
```

`ext` 里所有值都是**字符串**（含布尔写成 `"1"/"0"`、对象写成 JSON 串）。

### 3.1 `option`（44 个字段，按功能分组）

| 组 | 字段 |
|---|---|
| 会话 | `need_create_conversation:true`、`conversation_init_option:{need_ack_conversation:true}`、`conversation_init_ext:{model_item_key:"0",reasoning_effort:"3",mode_id:"1"}`、`conversation_mode:1`、`message_storage_type:0`、`related_deleted_message_ids:{}` |
| 模型/模式 | `agent_mode:2`、`need_deep_think:0`、`answer_with_suggest:false`、`model_config:{model_item_key:"0",model_extra_params:{},reasoning_effort:3}`、`aggregate_params:{mention_skill_list:"[]",mention_plugin_list:"[]",mention_ext:"[{}]",conversation_mode:"1",mode_id:"1",model_item_key:"0",agent_mode:"2",reasoning_effort:"3",provider_id:""}` |
| 流式 | `sse_recv_event_options:{support_chunk_delta:true}`、`support_lazy_fetch_stream:true`、`disable_sse_cache:false`、`start_seq:0` |
| 重生成/编辑 | `is_regen:false`、`is_replace:false`、`resend_for_regen:false`、`no_replace_for_regen:false`、`regen_query_id:[]`、`edit_query_id:[]`、`regen_instruction:""` |
| 恢复 | `recovery_option:{is_recovery:false,req_create_time_sec:1792161291,append_sse_event_scene:0}` |
| 场景/来源 | `send_message_scene:""`、`scene_type:0`、`message_from:0`(=InputBox)、`from_suggest:false`、`is_from_click_option:false`、`is_from_click_softlink:false`、`select_text_action:""`、`is_select_text:false`、`client_report_scene` 由服务端回填 |
| 其它 | `create_time_ms:1792161291322`、`unique_key:"759a0fcd-…"`（幂等键）、`collect_id:""`、`is_audio:false`、`tts_switch:false`、`click_clear_context:false`、`is_ai_playground:false`、`is_old_user:true`、`general_task_param:{server_do_compare:true}`、`shared_app_name:""`、`shared_app_id:""`、`connector_info_list:[]` |

**`unique_key`** 是客户端 UUID，服务端回到消息 `ext.reply_unique_key` → 可做幂等/对账。

## 4. 响应：SSE 事件序列（完整一次对话）

`HTTP/2 200`，`content-type: text/event-stream`，`server: openresty`。
事件是**字符串名**，不是 `fyf` 的数字枚举（docs/01 §3 那份枚举属于 `/samantha/*` 协议）。

| `id` | `event` | 作用 |
|---|---|---|
| 0 | `SSE_HEARTBEAT` | `data:{}` 立即一个，保活 |
| 0 | `SSE_ACK` | **会话建立回执**，见 4.1 |
| 1 | `FULL_MSG_NOTIFY` | 用户自己那条消息的落库形态，见 4.2 |
| 4 | `STREAM_MSG_NOTIFY` | 回复消息**首包**（含第一帧文本 + meta），见 4.3 |
| 6,7,10,13,16,17,20,21,22 | `STREAM_CHUNK` | 对消息对象打补丁，见 4.4 |
| 9,12 | `CHUNK_DELTA` | **纯文本增量** `{"text":"…"}`，反代最该用这个 |
| 18,19,23 | `SSE_REPLY_END` | 三种 `end_type`，见 4.5 |
| — | `STREAM_ERROR` | **异常路径**（03/04 初次记录时未见）：`{error_code,error_msg,extra}`，`710022004`=风控要滑块、`710022002`=过频。其后仍跟 `SSE_REPLY_END{end_type:3}`。详见 [05](doubao2api-05-live-probe-results.md) §3 |

注意 `id` 有跳号（1→4→6→7→9→10→12→…）：跳掉的是客户端未消费/心跳类帧；
`last-event-id` 续传用的就是这套 id。

### 4.1 `SSE_ACK` —— 首轮会话由服务端创建（关闭缺口 #3）

`data` = `{query_list[], ack_client_meta{}, timeout_conf{}}`：

- `query_list[0]`：`{question_id:"<REDACTED_LEN17>", local_message_id:"<客户端UUID>", message_index:1}`
  → **客户端 UUID 与服务端 message_id 的映射在这里给出**。
- `ack_client_meta`：`{conversation_id:"<REDACTED_LEN17>", local_conversation_id:"local_…",
  conversation_type:3, section_id:"<REDACTED_LEN17>", conversation_info{…}}`
  → 请求里 `conversation_id:""` 留空、`need_create_conversation:true`，**服务端自动建会话并回 ID**。
  不需要额外调 `/alice/conversation/create`。
- `conversation_info.extra`（字符串化 JSON）里回填了服务端归一化的上下文：
  `inner_app_id:582478`、`inner_pc_version:2.32.5`、`inner_platform:"web"`（注意：与 query 的 `desktop` 不同）、
  `model_item_key:"0"`、`reasoning_effort:"3"`、`mode_id:"1"`、`is_default_name:"true"`、`inner_user_ip`、`local_conversation_id`。
- `timeout_conf`：`{answer_first_pending_time:180000, packet_interval_time:120000,
  max_retry_count:10, max_retry_duration_ms:21600000, retry_interval_ms:1000}`
  → **反代的超时/重试参数应当从这里的 `packet_interval_time=120s` 取值**，别自己拍。

### 4.2 `FULL_MSG_NOTIFY` —— 服务端侧的消息对象

`data.message` 关键字段：`content_type:9999`、`content`（**字符串化 JSON 数组**，与 `content_block` 同内容但精简）、
`content_block[]`（结构化，带 `is_finish:false`）、`index_in_conv:1`、`sec_sender`（不透明令牌，脱敏）、
`bot_reply_message_id:"0"`、`fetch_token:"<REDACTED_QID>"`。

`message.ext` 是**大头**（40+ 键），值得反代保留的：
`chat_ability:"{}"`、`llm_model_type:"38"`、`model_type:"38"`、`input_skill:"{\"skill_id\":\"0\",\"skill_type\":0}"`、
`agent_mode:"2"`、`use_deep_think:"0"`、`reply_mode`/`reply_unique_key`、`ugc_voice_id:"104"`、
`speaker_id:"zh_female_wenroutaozi_uranus_bigtts"`、`search_engine_type:"4"`、`use_content_block:"1"`、
`brief:"你好"`、`chat_next:"1"`、`review_scenario_id`、`cot_switch:"0"`。
`user_type:1` = 人类（回复侧是 `2`）。

### 4.3 `STREAM_MSG_NOTIFY` —— 回复首包

```
data = {
  content: {
    content_block:[{block_type:10000, block_id:"10482015590636290",
                    content:{text_block:{text:"你"}}, is_finish:false, patch_type:1}],
    content_status:100,
    ext:{ llm_intention:"seed_main", agent_id:"19457456070409730", agent_name:"豆包",
          model_id:"38", model_type:"38", llm_model_type:"38", seed_intention:"seed_main",
          is_pro:"0", is_general_task:"0", msg_scene:"rv_two_col",
          bot_state:{bot_id,bot_name,agent_name,agent_id,agent_psm:"flow.agent.shell",intent_agent_name:"Agent-Chat"},
          tea_tags_time_cost:{ack_cost_time:"164", first_packet_cost_time:"1584", …},
          perf_mark:{marks:[{ev_name:"answer_first_token",ev_type:"query_duration"}]},
          review_scenario_id:"4085066754", … },
    content_type:9999,
    tts_content:"你"
  },
  meta: { message_id:"<REDACTED_AID>", conversation_id, section_id,
          sender_id:"7338286299411103781"(=bot_id), user_type:2, create_time,
          index_in_conv:2, bot_reply_message_id:"<REDACTED_QID>", local_conversation_id },
  attr: { reply_unique_key }
}
```

- **一问一答是两条独立 message**：提问 `<REDACTED_QID>`(index 1, user_type 1)，
  回答 `<REDACTED_AID>`(index 2, user_type 2)，用 `bot_reply_message_id` 正向挂回提问。
- `ext` 里才给出**服务端真实解析的模型**：`model_type:"38"` / `model_id:"38"`，而客户端只传
  `model_item_key:"0"`。→ 反代做模型名映射时，输入是 `model_item_key`/`mode_id`/`reasoning_effort`，
  真实模型要从响应 `ext.llm_intention`/`model_type` 读。

### 4.4 `STREAM_CHUNK` —— 补丁协议

```
data = { message_id, patch_op: [ {patch_object, patch_type, patch_value} ] }
```

| `patch_object` | 含义（本例观测） |
|---|---|
| 1 | `content_block`（正文；末帧 `is_finish:true` 且 `text_block:{}` 空） |
| 3 | `content_status`（`patch_type:2` 清除类，`patch_value:{}`） |
| 50 | `ext`（`is_finish:"1"`、`has_suggest:"1"`、`sp_v2` 建议列表、`perf_mark`、`tea_tags_time_cost`） |
| 111 | `tts_content` |

`patch_type`：`1` = 追加/合并；`2` = 清除/置空。

> **重要（已由 05 修正）**：正文在 `patch_object:1` 里是**整块覆盖式**给出（`text:"你"` 然后 `text:"好"`），
> 不是累加。`CHUNK_DELTA` **不保证出现**——05 §2 的实测里一次正常对话完全没有 `CHUNK_DELTA`，
> 正文只在 `STREAM_CHUNK patch_object:1`。
> → 反代必须以 **`STREAM_CHUNK patch_object:1` 为权威正文通道**，
> `CHUNK_DELTA` 只当逐 token 加速（有则用，无则按块输出），
> 非流式模式用 `end_type:1` 的 `msg_finish_attr.brief` 兜底。
> `STREAM_CHUNK` 其余补丁（`tts_content`/`ext`/建议）走旁路解析。

建议列表 `ext.sp_v2` 是**整串替换**（每项都重发全量 JSON 串），逐条增长。

### 4.5 `SSE_REPLY_END` —— 三个 `end_type` 是三段式收尾

| `end_type` | data | 含义 |
|---|---|---|
| 1 | `{end_type:1, msg_finish_attr:{msgid, badge_count, read_badge_count, read_conv_version, pre_read_conv_version, brief:"你好呀！…"}}` | **正文完成**（`brief` = 全文，可直接当非流式结果） |
| 2 | `{end_type:2, answer_finish_attr:{has_suggest:true}}` | 回答完成、**还有建议要发** |
| 3 | `{end_type:3}` | **流真正结束**（建议发完） |

→ 反代的 `finish_reason` 应在 `end_type:1` 给出；`end_type:3` 才是关流时机。
若 `has_suggest:false`，预计只有 1 和 3。

## 5. 响应头里的可复用信息

| 头 | 用途 |
|---|---|
| `x-ms-token` | **服务端下发新的 msToken**（下一次请求 query 要带上）。轮转式 token，反代必须回写 |
| `x-tt-agw-login: 1` | 网关判定已登录（对照匿名时回 `0`） |
| `x-tt-e-b: 1` | ttnet 加密通道**已启用**标记 |
| `x-tt-logid` / `x-tt-trace-id` / `x-tt-trace-host` | 排障用；错误分支要带出来 |
| `cache-control: no-cache` | — |
| `set-cookie: bd_sso_hi3jfd` | 会话期滚动下发，反代需维持 cookie jar |
| `tt-idc-switch: 10000@<ts>` | IDC 灰度开关 |

## 6. 与 docs/01 的冲突与合并

| 项 | docs/01（静态推演） | 实测 |
|---|---|---|
| 端点 | `/samantha/chat/completion` | **`/chat/completion`**（两套协议并存，此为 alice 五段式） |
| 体结构 | `messages`/`completion_option`/`evaluate_option`+顶层 id | **`client_meta`/`messages`/`option`/`user_context`/`ext`** |
| 块字段 | `content_blocks_v2` | **`content_block`（单数）**，`block_type:10000`=文本 |
| 事件 | 数字枚举 `2001/2003/2005`(`fyf`) | **字符串名** `SSE_ACK/STREAM_CHUNK/CHUNK_DELTA/…` |
| 错误 | HTTP JSON `{code,msg}`；SSE `ERR` | 未见；`gateway-error` 是 SSE 帧（见 02） |
| 会话创建 | 待测 | **服务端在 `SSE_ACK` 里自动建** |
| 增量文本 | 待测 | **`CHUNK_DELTA`** |

**docs/01 不作废**：它描述的 `cfz`(SamanthaChatProtocolPlugin) + `kXo` + `fyf` 是另一条
`/samantha/chat/completion` 路径，前端仍在用（`event_id` 续传、`x_frontier_qos`、异步任务 `async/stream`）。
两条路的取舍见 §7。

## 7. 下一步实验（按收益排序，**都需要用真实账号，会消耗登录态**）

1. **省略 `a_bogus` + `msToken` 重放本请求** → 决定反代要不要实现签名。02 已证 `/alice/user/launch`
   不校验，但 `/chat/completion` 是更高价值端点，需单独确认。
2. **省略 `web_tab_id` / `fp` / `tea_uuid` / `local_permissions`** → 求出最小必需参数集。
3. 多轮（带 `conversation_id`）+ `agent_mode` 切换（1/2）→ 补 `mode_id`/`model_item_key` 语义。
4. 打 `/samantha/chat/completion` 同一句话 → 对比两套协议，决定反代实现哪条
   （倾向：`/chat/completion`，因为实测已通、`CHUNK_DELTA` 干净）。
5. 用 `last-event-id: <n>` 重连 → 验续传。

## 8. 安全提示

本次贴出的请求含**完整可用登录态**（`sessionid`/`d_ticket`/`sid_guard`/`LARK_SUITE_ACCESS_TOKEN`/
`feishu_dpop_keypair`/`passport_mfa_token`），且已落入会话记录。
→ 建议在豆包端**退出登录或改密**使其失效。本文档只存头名与长度。
