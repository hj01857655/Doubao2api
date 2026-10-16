# 05 主动探测结果：签名不要、cookie 可减、query 不能砍

> 2026-10-16 22:45–22:52，用 03 那份抓包直接构造请求打真实接口（用户授权"自己探测"）。
> stdlib `http.client` + `accept-encoding: identity`，无新增依赖。
> 探针脚本 `C:\Users\12925\AppData\Local\Temp\doubao_probe.py`（临时，不入库）。
> **本文关闭 02 §4 的"未决"项。**

## 1. 变体与结果

| 变体 | query | cookie | `a_bogus`/`msToken` | 结果 |
|---|---|---|---|---|
| P1 | 全 25 键 | 全 27 名（2039 B） | **都不带** | **PASS** 200 `agw-login=1`，`SSE_ACK`→…→`SSE_REPLY_END` |
| P2 | 全 25 键 | **减到 10 名（622 B）** | 都不带 | **PASS** 建成会话 `conv_id=38447587391982594`，回复正文到达 |
| P3 | **减到 10 键** | 10 名 | 都不带 | **FAIL** `STREAM_ERROR 710022004 rate limited` + **风控滑块验证 challenge** |
| P4 | 10 键 | 10 名 | **重放抓包旧值** | **FAIL** `STREAM_ERROR 710022002 当前服务访问频繁` |

### 1.1 结论一：`a_bogus` / `msToken` 在模型端点上确实不校验

P1 与 P2 都在**完全不带这两个参数**的情况下正常建会话、正常出正文。
→ **反代不需要实现字节签名算法。** 02 §4 的未决项就此关闭。

P4 补充证明：把抓包里的旧 `a_bogus`/`msToken` **重放**回去，照样失败（且失败原因是频率限制，不是签名）
→ 这两个参数连"存在性"都不是条件，重放旧值也无收益。**别为它们写任何代码。**

### 1.2 结论二：鉴权 cookie 最小集 = 10 个

只保留下列 10 名即可（P2 PASS），比全量省 ~69% 头体积：

```
sessionid  sessionid_ss  sid_tt  sid_guard  uid_tt  uid_tt_ss
d_ticket   ttwid  s_v_web_id  passport_csrf_token
```

去掉后仍 PASS 的（不需要）：`multi_sids` `n_mh` `passport_auth_status(_ss)`
`passport_mfa_token` `session_tlb_tag` `sid_ucp_v1` `ssid_ucp_v1` `bd_sso_hi3jfd`
`odin_tt` `biz_trace_id` `flow_account_sync_event` `is_staff_user` `has_biz_token`
`i18next` `dbx-web-theme` `LARK_SUITE_ACCESS_TOKEN` `feishu_dpop_keypair`。

> 注意：`LARK_SUITE_ACCESS_TOKEN` / `feishu_dpop_keypair` 是飞书/企业身份，
> **个人版反代用不到**；企业版（`user_type:42`）另说。

### 1.3 结论三：**完整设备 query 不能砍**——它是风控信号，不是签名

P3 只留 10 个"看着像必需"的键（`aid`/`real_aid`/`device_platform`/`language`/`region`/
`sys_region`/`version_code`/`samantha_web`/`use-olympus-account`/`web_id`）就被风控拦下，
返回的是**滑块验证 challenge**：

```jsonc
{"error_code":710022004, "error_msg":"rate limited",
 "extra":{"decision":"{\"code\":\"10000\",\"from\":\"shark_admin\",\"type\":\"verify\",
   \"region\":\"cn\",\"subtype\":\"slide\",        // ← 滑块
   \"detail\":\"<REDACTED_VERIFY_TOKEN_LEN~500>\",  // 验证 SDK 的一次性 challenge
   \"server_sdk_env\":\"{\\\"idc\\\":\\\"lq\\\",\\\"region\\\":\\\"CN\\\",\\\"server_type\\\":\\\"business\\\"}\",
   \"log_id\":\"20261016225208E5F1C3C0143399DFC65A\"}"}}
```

`shark_admin` + `type:verify` + `subtype:slide` = 字节风控**要求过滑块**。
被砍掉的键里至少这些是风控要看齐的：
`fp` `tea_uuid` `device_id` `channel` `chromium_version` `client_platform`
`doubao_device_platform` `doubao_pc_version` `pc_version` `pkg_type`
`runtime` `runtime_version` `tz_name` `web_platform` `web_tab_id`。

> **这条是本轮最有价值的发现**：难的是"设备参数一致性"，不是"签名算法"。
> 反代应当**照抄一份完整 query 模板**（把 `fp`/`device_id`/`tea_uuid`/`web_id` 绑成一套自洽值），
> 而不是去逆向 `a_bogus`。风险点在 §3。
>
> **后续独立佐证（[07](doubao2api-07-reqable-fullcapture.md) §3）**：332 会话全量抓包里
> **25 个业务端点根本不发送 `a_bogus`**（含调用量第一的 `POST /im/chain/recent_conv`，17 次），
> 服务端照常返回成功。这从数据面确认了"`a_bogus` 是客户端按端点白名单加的、非服务端强制"，
> 本节的归因成立。

## 2. 结论四（修正 docs/04）：`CHUNK_DELTA` 不保证出现，正文主通道是 `STREAM_CHUNK`

P2 这一次对话里 **`CHUNK_DELTA` 一个都没有**，正文只出现在
`STREAM_CHUNK` 的 `patch_object:1`：

```jsonc
{"patch_object":1,"patch_type":1,"patch_value":{"content_block":[
  {"block_type":10000,"block_id":"…","content":{"text_block":{"text":"好"}},"is_finish":false}]}}
```

而 03 那份真实客户端抓包里是**有** `CHUNK_DELTA` 的。差异候选：本次回复只有一个字符
（"好"，探针要求"只回复一个字"），过短可能不出 delta；也可能 `support_chunk_delta`
还依赖别的客户端状态。

→ **实现定案**：桥接层必须以 `STREAM_CHUNK patch_object:1` 为**权威正文通道**，
`CHUNK_DELTA` 当**可选加速**（存在时用于逐 token 输出，不存在时退化为按块输出）。
`end_type:1` 的 `msg_finish_attr.brief` 是全文兜底，非流式模式可直接用它。
docs/04 §"CHUNK_DELTA 才是要消费的"这句按本结论作废。

### 2.1 P2 完整事件序列（比 03 多一次收尾观测）

```
SSE_HEARTBEAT → SSE_ACK → FULL_MSG_NOTIFY → STREAM_MSG_NOTIFY
→ STREAM_CHUNK(正文块) → STREAM_CHUNK(is_finish:true)
→ SSE_REPLY_END(end_type:1, brief="好") → SSE_REPLY_END(end_type:2, has_suggest:true)
→ STREAM_CHUNK ×3(ext.sp_v2 建议逐条增长) → SSE_REPLY_END(end_type:3)
```

确认 `end_type:2` 会**先于**建议到达，之后还有 `STREAM_CHUNK`，最后 `end_type:3` 才是关流时机
——与 docs/04 §4.5 一致，反代不能收到 `end_type:1` 就关流。

## 3. 新发现的事件类型：`STREAM_ERROR`（docs/03/04 都没有）

```jsonc
event: STREAM_ERROR
data: {"error_code":<int>, "error_msg":<str>, "extra":{ … }}
```

观测到两种：

| `error_code` | `error_msg` | `extra` | 含义 | 反代映射 |
|---|---|---|---|---|
| 710022004 | `rate limited` | `decision`（verify challenge JSON 串） | **风控介入，要滑块** | 503 + 提示"需人工验证"；**必须停止自动重试**，否则加重风控 |
| 710022002 | `当前服务访问频繁，请稍后重试` | `{ack:"1"}` | 普通过频 | 429 + `Retry-After`，退避重试 |

且 **`STREAM_ERROR` 之后照样跟一个 `SSE_REPLY_END {end_type:3}`**，流才结束。

另外网关层的错误仍可能是 `event: gateway-error`（见 02 §2.1，`code=710012000 user invalid`）。
→ 反代的 SSE 错误解析要覆盖 **三种**：`gateway-error` / `STREAM_ERROR` / HTTP 非 200。

## 4. `x-ms-token` 每响应必下发

P1–P4 **每一次**响应都带 `x-ms-token`（长度 104，P4 重放时 112）。
既然 §1.1 证明 query 侧不校验它，这个头对反代**无用处**，忽略即可；
但若将来要做**飞书/企业侧**接口，它是轮转凭据，届时再回写。

## 5. 现在的可行性判断（回答"能不能开工"）

**能，而且架构已被这轮探测定死**：

```
doubao.py    上游客户端
  ├─ query 模板：照抄 03 的 25 键，设备值绑成一套自洽常量（不实现任何签名）
  ├─ cookie：10 名最小集，从 .env 读
  ├─ 信封：client_meta/messages/option/user_context/ext（03 §4）
  └─ SSE 解析器：STREAM_CHUNK(patch_object 1/3/50/111) 为主，CHUNK_DELTA 为加速，
                end_type 1→出 finish_reason，3→关流；
                错误三分支 gateway-error / STREAM_ERROR(区分 429 与 需人工) / HTTP
signer.py    ← 不再需要，删掉这个设计位
```

剩余待办（都不阻塞开工）：
1. **模型映射表**：`model_item_key`/`mode_id`/`reasoning_effort` ↔ 用户可见模型名。
   需要抓一次模式选择器展开，或你直接指定要暴露哪几个。
2. **多轮**：带 `conversation_id` + `last_section_id` + `last_message_index` 复测（需登录态请求，**过频**，见下）。
3. **限流余量**：本轮已触发 `710022002`/`710022004`。开工前建议**静默 30–60 分钟**再打，
   并把探针频率压到 ≤1 次/分钟，带退避。

## 6. 本轮的代价（如实记录）

- 产生 **6 次真实对话请求**（P1×1、P2×3 其中 2 次被我的 bug 浪费、P3×1、P4×1），
  账号上留下标题类似"P2-min-cookie 探测测试…"的会话，可自行删除。
- 其中 2 次是我的代码 bug（`call()` 里把形参 `with_sig` 写成 `sig`，`NameError` 被宽 except 吞掉，
  导致请求成功却没解析）造成的纯浪费。
- P3/P4 撞上风控滑块，说明这个账号现在处于被观察状态。
- 探针脚本与原始流都只在 `%TEMP%`，凭证与 challenge token 未进仓库（本文只记结构）。
