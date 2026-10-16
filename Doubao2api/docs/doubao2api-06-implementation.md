# 06 反代首版实现与验证记录

> 2026-10-16 23:10–23:20。首版代码落地并完成离线 + 真实双验证。
> 代码：`app/doubao.py`（上游客户端）、`app/` 分层包 + `main.py`（协议壳）、`tests/`。
> （2026-10-17 目录重构：原 `server.py` 按社区 doubao2api 惯例拆为
> `app/{config,gateway,usage,routes,bridges}` + 薄入口 `main.py`，行为不变。）

## 1. 落地形态

```
Doubao2api/
├── main.py               薄入口（python main.py）
├── app/
│   ├── config.py         .env 加载 / 日志 / 鉴权配置
│   ├── gateway.py        数据面：客户端装配、重试、会话缓存、Chat 形状装配
│   ├── doubao.py         上游客户端：设备 query 模板 + 五段式信封 + SSE 状态机
│   ├── usage.py          token 用量估算
│   ├── routes/           /v1/chat/completions + /v1/messages + /v1/responses + /v1/models
│   └── bridges/          anthropic_bridge.py + responses_bridge.py（Chat ⇄ 各协议，纯 stdlib）
├── requirements.txt      只有 flask（.venv 已自带；其余全 stdlib）
├── .env                  10 个必需 cookie + 设备常量（不入库）
└── tests/
    ├── fixture_capture.txt        docs/03 真实响应的逐帧复刻
    ├── test_parser.py             离线 52 项：SSE 解析 + 请求面构造
    ├── test_error_paths.py        离线  8 项：错误路径回归（§7）
    ├── test_bridges.py            离线 26 项：两个协议桥（§7.3）
    ├── verify_stream_multiturn.py 真实流式 + 多轮验证
    └── verify_latency.py          真实时延验证
```

**零签名代码**（05 §1.1 定了这条）。`.env` loader、SSE 解析、HTTP 客户端全部 stdlib。

## 2. 模型映射（零请求，从已有样本解出）

来源：`/alice/slot/action_bar_v3/brief_list` 响应的
`data.entry_list[0].active_switch_conf.menu_conf_v2.model_list.item_list[*]`
（从 `tools/samples/195043-039-…resp.bin` 解出，未新增任何请求）。

| 暴露名 (`model`) | `model_item_key` | `mode_id` | 显示名 |
|---|---|---|---|
| `doubao-fast` | `0` | 1 | 豆包 快速 |
| `doubao-expert` | `3` | 1 | 豆包 专家 |
| `doubao-auto` | `9` | 3 | 自动 |
| `doubao-turbo` | `4` | 3 | 豆包 2.1 Turbo |
| `doubao-pro` | `5` | 3 | 豆包 2.1 Pro |
| `doubao-2.1-lite` | `seed-lite-7b` | 3 | 豆包 2.1 Lite |

默认档 `doubao-fast`；别名 `doubao`/`auto`/`fast`/`expert`/`doubao-seed` 可用；
也可直接把 `model_item_key` 当模型名传。
`reasoning_effort` 固定 `"3"`（抓包默认值）。
`default_select_model_map`：`chat→0`、`office→9`、`creation→9`。

## 3. 实现期发现并修正的两处错误认知

### 3.1 `brief` 不是全文，是会话列表预览串（会吃换行）

真实观测（`server.log`）：

```
正文(assembled) = "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"
brief           = "123456789101112"          ← 换行被吃掉
```

05 §2 与 04 §4.5 里我写的"`end_type:1` 的 `msg_finish_attr.brief` 是权威全文兜底"**作废**。
首版代码一度按 `len(brief) >= len(assembled)` 择优，长答案/带换行的答案会被预览串覆盖。

**定案**：`text` 恒等于拼接结果；`brief` 只作独立预览字段返回，比较时须先折叠空白。
已加回归用例（`tests/test_parser.py` T2b）。

### 3.2 `finish_reason` 不该等 `end_type:3`

实测正文说完（`end_type:1`）到建议发完（`end_type:3`）之间差 **2–3 秒**。
首版把 `finish_reason` 压在 `end_type:3` 后发，客户端白等。
现在解析器在 `end_type:1` 额外产出 `body_end` 事件，`app/routes/openai_chat.py` 立即发 `finish_reason`，
后台继续把建议帧读完再发 `[DONE]`。

时延实测（`tests/verify_latency.py`，一次真实请求）：

```
first_delta    3.80s
last_delta     6.73s
finish_reason  6.73s      ← 落后末片 0.00s（修复前约 2.6s）
[DONE]         6.73s
```

## 4. 验证结果

### 4.1 离线（`tests/test_parser.py`，零网络）

**PASS 52 / FAIL 0**。覆盖：

- 逐帧复刻 docs/03 真实响应 → 片段序列、拼接正文、三条正文通道识别、
  `conversation_id`/`question_id`/`answer_message_id`/`model_type`/建议全量取值、
  `tts_content` 镜像通道未混入正文。
- 无 `CHUNK_DELTA` 形态（05 §2 的 P2 情形）→ 仅靠 `patch_object:1` 仍出正文，
  且不被 `brief` 覆盖。
- 错误三分支 → 风控滑块 `710022004`⇒`risk`/不可重试；过频 `710022002`⇒`rate`/429/可重试；
  `gateway-error 710012000`⇒`auth`/401/不可重试。
- 请求面 → query 不含 `a_bogus`/`msToken`、设备参数齐全且 `fp==verify_<device_id>`、
  `tea_uuid==device_id`；首轮/续轮 `need_create_conversation` 切换；
  `ext` 值全字符串；cookie 裁到 10 名且不含 LARK 令牌；缺必需 cookie 立即报错。

### 4.2 真实端到端（共 4 次请求）

| 验证 | 结果 |
|---|---|
| 非流式 | `200`，`content="收到"`（服从"只回复两个字"），带回 `conversation_id`/`question_id`/`answer_message_id`/`model_type="38"`/`suggestions[3]`/`log_id` |
| 流式增量 | 82 字答案，`first_delta 3.80s`→`last_delta 6.73s`，跨 3 秒逐块到达 ⇒ **确为增量转发** |
| 多轮续接 | 带 `X-Doubao-Conversation-Id` 复问"我上一句让你做什么"，模型准确复述上一轮指令与自己的回复 ⇒ **服务端持有历史，续接有效** |
| 模型/会话号回传 | 响应 `doubao.*` 字段与响应头 `X-Doubao-Conversation-Id` 一致 |

一次短答案（"从1数到12"）里 6 个片段在同一时刻到达，属于该答案本就一次说完，非转发退化；
增量能力由上面 82 字答案证实。

## 5. 已知限制

1. **`usage.tokens` 是字符数估算**（豆包不回 token 数），代码里已注明。
2. **新会话才带历史**：已有 `conversation_id` 时只发最新一条 user 消息，历史由豆包服务端持有；
   若调用方每轮都传全新上下文，会与豆包服务端历史叠加。续接由调用方掌握
   （`X-Doubao-Conversation-Id` 或 `user` 字段）。
3. **多模态未实现**：只支持 `block_type:10000` 文本块；图片/文件/技能引用未接。
4. **建议(`sp_v2`)透传但非 OpenAI 标准字段**，放在 `doubao.suggestions`。
5. **`agw-js-conv` 按单值 `str` 发**（抓包是 `"str, str"`），实测可用。
6. 单账号串行锁（`threading.Lock`）——并发请求排队，避免同 cookie 并发触发过频。
7. Flask dev server，未做生产化（README 纪律：协议验证 ≠ 生产使用）。

## 6. 本轮真实账号足迹

累计 **约 13 次**真实请求（05 探测 9 次 + 本文验证 4 次），账号上留下多条探针会话
（标题含"探测测试"、"从1数到12"、"用一句话介绍长城"等），请自行清理。
其中 05 触发过 `710022004`（风控滑块）与 `710022002`（过频），
**账号处于被观察状态，继续联调请限频**（代码已单账号串行 + 仅对 `rate` 退避一次，
遇 `risk` 立即失败不重试）。

## 7. 补修：错误路径的两个真 bug（真实联调打不到，靠离线逼出）

这两条都在"上游中途报错"的分支上，正常成功路径永远碰不到，是 `tests/test_error_paths.py`
用假客户端逼出来的。

### 7.1 已发正文后仍退避重试 → 正文重复下发

`_backoff_stream` 原来是 `yield from CLIENT.stream(...)`，捕获可重试错误就整段重跑。
问题是：**已经吐给下游的 delta 会在重试时被第二次重放**，客户端拿到
`你好，世界你好，世界` 这种重复文本。

实测复现（假客户端：第一次发 2 个 delta 后抛 `710022002`）：

```
上游被调 2 次, deltas=['你好', '，世界', '你好', '，世界']    ← 重复
```

修法：**一旦发出过任何内容就绝不重试**，直接把错误抛给上层（流式路径转为 SSE error 帧收尾）。

```
上游被调 1 次, deltas=['你好', '，世界'], raised=rate limited   ← 正确
```

代价：中途过频不再自愈。这是对的取舍——宁可少一个字也不要重复一个字。
重试只在"尚未输出任何内容"时才有意义（连接阶段就 429 的情形仍会退避一次）。

### 7.2 流式生成器里调用 `jsonify()` → 直接崩

`/v1/chat/completions` 的流式错误分支里写了 `fail_from(exc)`，而 `fail_from` 返回
`jsonify(...)`。Flask 在视图函数返回后就**弹出了请求上下文**，生成器体是之后才执行的，
于是：

```
RuntimeError: Working outside of application context.
```

即风控/身份错误一旦发生在流式模式下，客户端拿到的是断掉的流而不是错误帧。

修法：新增不依赖上下文的 `_err_type(exc)` 映射，生成器内直接 `yield` 纯 dict：

```json
data: {"error":{"message":"风控要求验证 type=verify subtype=slide",
               "type":"risk_control_error","code":710022004,"http_status":503}}
data: [DONE]
```

另外两个桥（`/v1/messages`、`/v1/responses`）走 `_chat_sse_stream`，本来就正确地构造 dict，
无此问题；已把那里的 `type` 也统一用 `_err_type(exc)`（原先漏成裸 `exc.kind`）。

### 7.3 桥接层验证（离线，零真实请求）

`tests/test_bridges.py` 用假上游喂固定 `done` 帧，覆盖四个模式 + 两个流式错误路径：**26/26**。
观测到的输出形状：

- `/v1/messages` 非流式：`{content:[{type:text,text:…}], role, stop_reason, usage, doubao{…}}`
  + 响应头 `X-Doubao-Conversation-Id`。
- `/v1/messages` 流式事件：`message_start → content_block_start → content_block_delta →
  content_block_stop → message_delta → message_stop`；错误时插 `event: error`。
- `/v1/responses` 非流式：`{object:"response", status:"completed", output:[…output_text…]}`。
- `/v1/responses` 流式事件：`response.created → response.in_progress →
  response.output_item.added → response.output_text.delta → response.output_text.done →
  response.output_item.done → response.completed`；错误挂在 `response.error`。

> 我起初断言"错误后正文不得重复"在 Responses 上误报失败——`response.output_item.done`
> 按规范**本来就要携带完整文本**，与前面的 delta 并存是正确行为。断言已收窄到只查 delta 事件。

## 8. 离线测试总账

```
tests/test_parser.py       PASS 52   FAIL 0    SSE 解析 + 请求面构造
tests/test_error_paths.py  PASS  8   FAIL 0    §7.1 / §7.2 回归
tests/test_bridges.py      PASS 26   FAIL 0    两个协议桥 + 流式错误路径
                           ─────────────────
                           86 项断言，全部零网络
```

服务已在重启后加载当前代码：`/health` 200、`/v1/models` 200（6 档）。

> 2026-10-17 修正：`/v1/models` 在 `server.py` → `app/routes/` 目录重构时丢失（实测 404），
> 已补为 `app/routes/openai_models.py`（提交 `0704838`）；上面这行断言在补回后才成立。

### 8.1 真实上游补验（1 次请求）

`/v1/messages` 走真上游通过：`role=assistant`、`stop_reason=end_turn`、
`content=[{type:text,text:"我很好"}]`（服从"只回复三个字"）、
`usage.input_tokens=5/output_tokens=1`、`doubao{conversation_id,question_id,answer_message_id}`
与响应头 `X-Doubao-Conversation-Id` 一致。

**仍未经真实上游验证的**：非默认模型档，以及全部错误分支
（**故意不打**——再触发一次 `710022004` 只会加重账号风控状态）。
`/v1/responses` 与 chat / responses 两种流式形态已于 2026-10-17 补验，见 §8.4。

### 8.2 用 Reqable 原始字节复核解析器（零新增请求）

从 `Reqable\capture\1792161279562512-10-119-res-raw-body.reqable`（10257 B，
即 03 那条 `/chat/completion` 的**原始响应字节**，非我复刻的 fixture）直接喂给解析器：

| 项 | 观测 |
|---|---|
| 帧数 | 18 |
| 事件构成 | `SSE_HEARTBEAT 1, SSE_ACK 1, FULL_MSG_NOTIFY 1, STREAM_MSG_NOTIFY 1, STREAM_CHUNK 9, CHUNK_DELTA 2, SSE_REPLY_END 3` |
| 解析出的正文 | `你好呀！有什么我可以帮你的吗？` |
| 与 `brief` 对比 | 逐字一致（`len=16`） |
| 用到的正文通道 | `stream_msg_notify` + `stream_chunk_patch1` + `chunk_delta`（三者互不重叠，拼接正确） |
| 会话/消息号 | `conv=38447554619021570` `q=58756254450320130` `a=58756254450326274` `model_type=38` |
| 建议 | 3 条 |

⇒ 证实 §3.1（`brief` 只在该次恰好等于全文，不可当通用兜底）与 §3.2 的事件时序，
并且是**独立数据源**验证，不依赖我自己写的 fixture。

### 8.3 完整抓包清单的落盘位置

- `tools/reqable_inventory.md` —— 332 会话去重后的**完整**清单：
  132 端点（含每个端点的 query 参数名与是否带 `a_bogus`）、56 个请求头名、
  108 个响应头名、125 个 query 参数名、状态码分布、关键判读。
- `tools/reqable_index.json` —— 332 条逐流索引（`sid/method/host/path/status`）。

两份都只含名字/计数/结构，**不含任何头值、cookie、token、query 值**。
分析正文见 [07](doubao2api-07-reqable-fullcapture.md)。

### 8.4 三协议 × 两形态实测矩阵（2026-10-17，共 6 次上游调用）

入站鉴权已启用（`.env` 的 `API_KEY`），下列调用均带 `Authorization: Bearer <key>`；
每次上游调用之间按 05 的纪律隔 60 秒。

| 协议 | 形态 | 实测结果 |
|---|---|---|
| `/v1/chat/completions` | 非流式（**带 `tools` 声明**） | 200；`message` 键只有 `content, role`，**无 `tool_calls`**——上游不认 `tools` 声明 |
| `/v1/chat/completions` | 流式 | 200；6 个 `data:` 帧，首帧 `delta.role=assistant`，正文拼接 `你好`，末帧 `[DONE]` |
| `/v1/messages` | 非流式 | 200；`type=message`、`stop_reason=end_turn`、`content[0].type=text` |
| `/v1/messages` | 流式 | 200；事件序列 `message_start → content_block_start → content_block_delta ×2 → content_block_stop → message_delta → message_stop` |
| `/v1/responses` | 非流式 | 200；`object=response`、`output[0].content[0].type=output_text`、usage 含 `reasoning_tokens` 明细 |
| `/v1/responses` | 流式 | 200；`response.created`（`sequence_number=0`）→ `response.completed`（`sequence_number=7`），终帧携带聚合全文与 usage |

入站鉴权矩阵（用 `/v1/models` 探，不耗上游）：

| 请求 | 结果 |
|---|---|
| 不带 key / 错 key | `401`，体为 `{"error":{"code":"invalid_api_key","type":"auth_error"}}` |
| `Authorization: Bearer <key>` / `x-api-key: <key>` | `200` |
| `/health` | `200`（不进 `guard()`，留作探活） |

> 目录说明：本节与 §8.3 引用的 `tools/…` 均指**仓库根**下的 `tools/`，不在 `Doubao2api/` 内；
> 该目录与 `workspace/` 同属不入库的本地取证面（见根 `README.md` 的 .gitignore 说明）。
