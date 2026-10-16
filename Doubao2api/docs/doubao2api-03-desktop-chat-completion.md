# 03 `/chat/completion` 桌面真实样本对照（Phase 1 定案）

> 来源：**一份已登录桌面端真实请求 + SSE 响应**（用户 2026-10-16 提供，非推断）。
> 本文**以实测为准**，凡与 `doubao2api-01`（前端 Web bundle 静态还原）冲突处，以本文为准。
> 脱敏纪律：cookie / msToken / a_bogus / LARK token / sessionid 一律**只记字段名 + 长度 + sha1[:8]**，值不落盘。

## 1. 端点（纠正 docs/01）

| 项 | 真实值 | 与旧判断 |
|---|---|---|
| method/path | `POST /chat/completion`（host `www.doubao.com`，HTTP/2） | docs/01 写 `/samantha/chat/completion`；匿名探测打后者也能进业务网关（回 `Invalid User ID`），**两条路由都存在**，桌面实际走不带 `/samantha` 前缀这条。**待第 2 轮确认** |
| 响应 | `text/event-stream`，`server: openresty`（非 Tengine，与 `/alice/*` 商务面不同网关） | 印证 SSE |
| 门禁标记 | 响应头 `x-tt-agw-login: 1`（登录态）；匿名态该值为 `0`/缺 | 登录判定实锤 |

## 2. Query 面 = 设备指纹 + 签名（**纠正我上一轮"无签名"的错误结论**）

请求 query 携带完整设备/签名参数集，**签名不在 header、在 query**：

| 键 | 样例形态 | 说明 |
|---|---|---|
| `aid` / `real_aid` | `582478` | 豆包桌面 app id |
| `channel` | `win` | |
| `device_id` / `tea_uuid` | `<REDACTED_LEN15>` | 同一值，设备主键 |
| `fp` | `verify_<device_id>` | 风控指纹（由 device_id 拼接，非独立熵源） |
| `web_id` | `<REDACTED_LEN19>` | |
| `web_tab_id` | uuid | 每次新标签变 |
| `device_platform` / `doubao_device_platform` | `web` / `desktop` | **注意 device_platform=web**（桌面也标 web） |
| `client_platform` / `web_platform` / `runtime` | `pc_client` / `desktop` / `web` | |
| `chromium_version` / `runtime_version` | `147.0.7727.149` / `3.41.2` | |
| `pc_version` / `doubao_pc_version` / `version_code` / `pkg_type` | `2.32.5` / `2.32.5` / `20800` / `release_version` | |
| `region` / `sys_region` / `language` / `tz_name` / `samantha_web` / `use-olympus-account` | `CN`/`CN`/`zh`/`Asia/Shanghai`/`1`/`1` | |
| **`msToken`** | `<REDACTED_LEN176>` 尾 `=`，url-safe 字符集 | **服务端签发**（见 §6） |
| **`a_bogus`** | `<REDACTED_LEN160>`（url-encode 后） | **客户端 webmssdk 计算**（见 §7，唯一未决风险） |

> 我之前"不带任何 x-tt-* 也能打到网关 → 无签名"是**误读**：那只能证明**身份校验(msToken/uid)在签名校验之前**，不能证明签名不被校验。

## 3. 请求头

```
agw-js-conv: str, str          # 双值
last-event-id: undefined       # JS 未初始化，字面 "undefined"，服务端容忍（反代可发 "0" 或省略）
user-agent: …Chrome/147… SamanthaDoubao/2.32.5    # 产品标识尾缀
content-type: application/json
accept: */*
origin: https://www.doubao.com      # 有 origin（与部分 /alice 匿名请求不同）
referer: https://www.doubao.com/
sec-fetch-site: cross-site          # 注意：cross-site（同域却标 cross-site，桌面 webview 特性）
cookie: <会话态，见 §5>
```
**无独立 Authorization 头**——鉴权全在 cookie。

## 4. 请求体（桌面真实信封，与 docs/01 的 Web 信封不同）

顶层 5 键：`client_meta` / `messages` / `option` / `user_context` / `ext`。

```jsonc
{
  "client_meta": {
    "local_conversation_id": "local_2072112497206180",  // 客户端造
    "conversation_id": "",                              // 首轮留空 → 服务端 ACK 回填
    "bot_id": "7338286299411103781",                    // "豆包" 主 bot
    "last_section_id": "", "last_message_index": null,  // 续聊时回填（待第2轮证实）
    "local_permissions": [ {"permission_name":"ACCESS_COARSE_LOCATION","status":1}, … GPS 三件套 ]
  },
  "messages": [ {
    "local_message_id": "<uuid v1>",
    "content_block": [                                  // 单数 block（docs/01 写的是复数 content_blocks_v2）
      { "block_type": 10000,                            // 10000 = 纯文本块
        "content": { "text_block": { "text":"你好","icon_url":"","icon_url_dark":"","summary":"" },
                     "pc_event_block":"" },
        "block_id":"<uuid>", "parent_id":"", "meta_info":[], "extra_info":{}, "append_fields":[] }
    ],
    "message_status": 0
  } ],
  "option": {                                           // 对应 docs/01 的 completion_option，字段名不同
    "need_create_conversation": true,
    "conversation_init_option": {"need_ack_conversation": true},
    "conversation_init_ext": {"model_item_key":"0","reasoning_effort":"3","mode_id":"1"},  // 选模型
    "model_config": {"model_item_key":"0","reasoning_effort":3,"model_extra_params":{}},
    "aggregate_params": {"conversation_mode":"1","mode_id":"1","agent_mode":"2","model_item_key":"0","reasoning_effort":"3", …},
    "agent_mode": 2, "message_from": 0,                  // message_from 是**整数**（docs/01 说是字符串枚举，错）
    "create_time_ms": 1792161291322, "unique_key":"<uuid>",
    "sse_recv_event_options": {"support_chunk_delta": true},   // ← 开了才有 CHUNK_DELTA 事件
    "support_lazy_fetch_stream": true,
    "recovery_option": {"is_recovery":false,"req_create_time_sec":…,"append_sse_event_scene":0},
    "is_regen":false,"is_replace":false,"answer_with_suggest":false,"need_deep_think":0,
    "is_old_user":true,"disable_sse_cache":false,"scene_type":0,"general_task_param":{"server_do_compare":true}, …
  },
  "user_context": [],
  "ext": { "agent_mode":"2","use_deep_think":"0","conversation_init_option":"{…}",
           "commerce_credit_config_enable":"0","is_finish":"1","sub_conv_firstmet_type":"1", … }  // ext 多为字符串化 JSON
}
```

## 5. 会话态 cookie（**只列名，值不落盘**）

承载登录身份的 cookie（缺任一即 `Invalid User ID`）：

| cookie | 角色 | 指纹(len/sha1[:8]) |
|---|---|---|
| `sessionid` / `sessionid_ss` | 主会话票 | 32 / 见 bin |
| `sid_tt` / `uid_tt` / `uid_tt_ss` | 会话/用户 id | |
| `sid_guard` | `sid\|签发ts\|寿命秒\|到期GMT`（寿命 `2591999`≈30天） | |
| `sid_ucp_v1` / `ssid_ucp_v1` | base64 封装的会话+用户 | |
| `d_ticket` / `n_mh` / `bd_sso_hi3jfd` | SSO 支撑票 | |
| `passport_auth_status(_ss)` | md5 登录态标记 | |
| `passport_mfa_token` | 长票（MFA） | |
| `multi_sids` | `uid:sid` | |
| **`LARK_SUITE_ACCESS_TOKEN`** | **JWT**（ES256）payload 含 `doubao_uid`/`device_id`/`auth_time`/`exp`；本票 `exp-iat`=5天 | |
| `ttwid` / `s_v_web_id` / `odin_tt` / `fp` | 设备/匿名基线票 | |
| `passport_csrf_token(_default)` | CSRF（POST 是否必需待测） | |

> `bd_sso_hi3jfd` 会在响应 `Set-Cookie` 轮换（Domain=doubao.com; HttpOnly; Secure; SameSite=None）。

## 6. SSE 响应协议（**推翻 docs/01 的 event_type 数字枚举模型**）

真实协议 = **命名事件 `event: <NAME>` + `id: <单调序号>` + `data: <json>`**，不是 `{event_type:2001}` 那套。生命周期：

| 顺序 | event | 载荷要点 | 用途 |
|---|---|---|---|
| — | `SSE_HEARTBEAT` | `{}` | 保活 |
| id0 | `SSE_ACK` | **`ack_client_meta`：服务端 mint `conversation_id`=`<REDACTED_LEN17>` / `section_id`=conv_id+256 / `conversation_type`=3** + 完整 `conversation_info`；`query_list[{question_id,local_message_id,message_index}]`；`timeout_conf{answer_first_pending_time:180000, packet_interval_time:120000, max_retry_count:10, retry_interval_ms, max_retry_duration_ms}` | 建会话 + **客户端看门狗参数** |
| id1 | `FULL_MSG_NOTIFY` | 回显**用户**消息落库形态（`content_type:9999`、`content` 为 block 的 JSON 串、`content_block[]`、`fetch_token`） | 确认上行已存 |
| id4 | `STREAM_MSG_NOTIFY` | **首个助手包**：mint 助手 `message_id`=`<REDACTED_LEN17>`（与提问 id 同前缀段）/ `index_in_conv`=2 / `sender_id`=bot_id / `user_type`=2；`content.content_block[]`（首字 "你"）；`ext` 巨量元数据：`llm_intention:seed_main`、`agent_id:19457456070409730`、`model_type:38`/`llm_model_type:38`、`msg_scene:rv_two_col`、`bot_state`、`perf_mark`、`tea_tags_time_cost{ack_cost_time,first_packet_cost_time:1584}` | 建立下行消息壳 |
| id6/16… | `STREAM_CHUNK` | `message_id` + `patch_op[]`：`{patch_object, patch_type, patch_value}` | 增量补丁（见下） |
| id9/12… | **`CHUNK_DELTA`** | `{"text":"呀！有什么我可以"}` | **纯文本增量**——反代映射 OpenAI `delta.content` 首选通道 |
| id18 | `SSE_REPLY_END` | `end_type:1` + `msg_finish_attr{msgid, brief:"整句", badge…}` | 该消息正文结束 |
| id19 | `SSE_REPLY_END` | `end_type:2` + `answer_finish_attr{has_suggest:true}` | 回答结束（可能带追问） |
| id20-22 | `STREAM_CHUNK` | `patch_object:50 → ext.sp_v2`（追问建议数组，逐条累加） | suggest 流 |
| id23 | `SSE_REPLY_END` | `end_type:3` | **整轮终止** |

**`patch_object` 字段选择器**（由样本归纳）：

| patch_object | 目标 | patch_type | 观察 |
|---|---|---|---|
| `1` | `content_block[]`（正文块） | 1=追加/合并；末帧给 `text_block:{}`+`is_finish:true` 收尾 | 正文走这里 |
| `111` | `tts_content` | 1 | 语音播报流（可忽略） |
| `50` | `ext`（消息级扩展：`is_finish`/`perf_mark`/`has_suggest`/`sp_v2`） | 1 | 状态/追问走这里 |
| `3` | 未定 | **2**（清空/删除，`patch_value:{}`） | 语义待测 |

> **正文重组**：`CHUNK_DELTA.text` 串起来 = "你好呀！有什么我可以帮你的吗？😊"（与 brief 一致）→ **反代直接取 CHUNK_DELTA 累加即得 OpenAI 内容**，不必解析 patch。`id` 单调递增 = 断线续传游标（回 `last-event-id`）。

## 7. 反代落地要点 + 唯一未决风险

**已足够写骨架**：端点/头/体信封/SSE→OpenAI 映射（CHUNK_DELTA→delta.content，SSE_REPLY_END end_type:3→finish_reason，SSE_ACK→conversation_id 续聊存储）全部有据。

**模型标识**：本样本 default 豆包 = `bot_id=<REDACTED_LEN19>`、`model_id/model_type=38`、`reasoning_effort=3`、`mode_id=1`、`model_item_key="0"`。其它模型的 `model_item_key`/`reasoning_effort`/`mode_id` 取值 = 待补枚举。

**msToken（§5 query）**：响应头 **`x-ms-token`** 下发**下一个** msToken → 反代可从上一轮响应捕获并回填下一轮 query，**可自持**。

**⚠ 唯一硬风险 = `a_bogus`**：客户端 webmssdk 生成、每请求变。本样本证明了它的**存在**，但没证明**服务端强校验**。三种可能，决定 2api 可行性：
1. 软校验（缺了也放行，只要 msToken+cookie 对）→ 反代可 headless；
2. 强校验但算法可离线复现（webmssdk 是 JS，可搬）→ 反代内嵌那段 JS；
3. 强校验且绑定设备指纹/时间窗动态 → 只能驱动真实客户端取 a_bogus。

**关闭 §7 风险 + §1/§4 续聊字段的决定性一步**：拿**这份 cookie + query 面**，用**同一会话**：
- (a) 发第 2 轮（`need_create_conversation:false`、回填 `conversation_id`/`last_section_id`）→ 补续聊信封；
- (b) 故意**去掉 a_bogus**（其余照旧）再打一次 → 直接判 §7 落在 1/2/3 哪一档。
两次都是**单条最小请求**，不轮询、不建持续抓包（合工程纪律 §2）。

## 8. 复现/取证命令（已验证链路）
```powershell
cd E:\VSCodeSpace\Agent\Doubao\Doubao2api
.\.venv\Scripts\python.exe tools\mitm_addon.py          # 零参数全量抓包(8080)
pwsh tools\run_desktop.ps1 -UseRealProfile              # 已登录客户端走代理（一次即可）
# 判读： tools\samples\manifest.jsonl 里 grep chat/completion → 其 raw *.bin 即本文样本来源
```
> 本 `doubao2api-03` 的样本由用户手工导出提供，非本仓库自动落盘（真实 cookie 值始终未写入仓库）。
> 注：`doubao2api-04-sse-response-deepdive.md`（并行作者）§4.1 仍残留一处**未脱敏**的 `conversation_id` 字面值，待抹。
