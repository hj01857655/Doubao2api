# 02 HTTP 侧无签名头；`a_bogus` 仅在 `/alice/*` 被证伪

> 实测时间 2026-10-16 21:4x（本机直连 `www.doubao.com`，`Invoke-WebRequest`，非浏览器）。
> **本文件的结论已被 [03](doubao2api-03-desktop-chat-completion.md) 收紧**：
> 「请求头无签名」成立；「`a_bogus` 不校验」只在 `/alice/user/launch` 成立，
> 在模型端点上属未决。原标题「签名关卡被实测证伪」过度外推，已改。

## 1. 实验设计

只带 `ttwid`（访问 `/chat/` 自动下发，127 字符，匿名设备态），不带任何登录 cookie、
不带 `x-tt-*` 签名头，比较三种输入的返回。

## 2. 结果

### 2.1 模型端点：签名没被拦，卡在用户身份

```
POST https://www.doubao.com/samantha/chat/completion
content-type: application/json     agw-js-conv: str
origin: https://www.doubao.com     referer: https://www.doubao.com/chat/
cookie: ttwid=<匿名>               body: {"messages":[],"completion_option":{"event_id":"0"}}
→ 200, Content-Type: text/event-stream

event: gateway-error
data: {"code":"Internal","message":"ErrorX:code=710012000 stable=true isPeer=false
       peerStable=false message=[user invalid]\nchain=[\n(cause)0: Invalid User ID\n]"}
```

要点：
- **请求穿过了 TLS、穿过网关、进到业务层**，业务层抱怨的是 `Invalid User ID`，不是签名。
- 响应是 **SSE**，错误也用 SSE 帧回（`event: gateway-error`），不是 JSON。
  → 反代的错误分支要按 SSE 解析，不能只按 HTTP status。
- `code=710012000` / `message=[user invalid]` 是可识别的错误码。

### 2.2 业务端点：带假签名与不带签名，返回完全一致

```
POST https://www.doubao.com/alice/user/launch?language=zh-CN&device_platform=webpc&aid=436&...
A) 不带 a_bogus      → 200, len=2644, code=0, msg=""
B) 带 a_bogus=d0mXs  → 200, len=2644, code=0, msg=""   ← 值是假的
```

同一长度、同一 `code=0`。**服务端不校验 `a_bogus` 的存在性，也不校验其正确性。**

## 3. 与抓包数据的交叉验证

`tools/samples/manifest.jsonl`（516 条带请求头的记录）：

| 观察 | 数值 | 含义 |
|---|---|---|
| `www.doubao.com` 的 POST 请求 | 169 条 | 请求头名全集只有标准浏览器头 + `agw-js-conv` + `cookie` |
| 出现过的签名类请求头 | 仅 `x-tt-passport-*`（6 条，passport 域） | 业务域 `www.doubao.com` 一个都没有 |
| query 里带 `a_bogus` 的 | 127 条 / 24 个端点 | 前端确实加了，但加了不等于服务端查 |
| query 里**不**带 `a_bogus` 的 doubao 业务请求 | 也存在（`/im/chain/recent_conv`、`/ttwid/check/`、`/samantha/user/ab/get` 等） | 同一批 API 有无签名都能通 |

前端侧：`a_bogus` 字面量**不在任何业务 bundle 里**，`webmssdk.es5.js`（1.0.0.43，321 KB）里
`a_bogus` 计数也是 0，它只产 `X-Bogus`（2 处）和 `msToken`（1 处）。`X-Bogus` 的实际用途只有一处：
`s2-chat-runtime.js` 里给 WebSocket `wss100-normal.doubao.com/ws/v2` 的 query 签名
（`byted_acrawler.frontierSign({"X-MS-STUB": stub})`），不是给 HTTP 用的。

## 4. 结论的**适用边界**（重要，勿过度外推）

本实验只证明了两件事，**不能**推广成"豆包不校验签名"：

1. **`/alice/user/launch` 这一个端点**对 `a_bogus` 既不查存在性也不查正确性（带假值 `code=0`）。
2. **`/samantha/chat/completion`** 上，身份校验排在签名校验**之前**——匿名请求先撞到
   `Invalid User ID` 就返回了，**签名有没有被检查、检查会不会失败，本实验无法判定**。

> 我最初把标题写成"网关不校验 a_bogus"是过度外推，已由
> [03 桌面真实样本](doubao2api-03-desktop-chat-completion.md) §2 纠正，此处按边界收紧。
> `a_bogus` 是否仍是 `/chat/completion` 的实际关卡，**属于未决风险**，见 03 §7 与本文 §5。

| 关卡 | 状态 | 依据 |
|---|---|---|
| TLS | 已通（标准 TLS 1.3） | DOUBAO-15 |
| HTTP 请求头签名 `x-tt-e-*`/`x-tt-s-*` | **确认不存在**（169 条 POST 头名全集无一例外；真实登录态请求也无） | 本文 §3 + 03 §3 |
| query `a_bogus` 在 `/alice/user/launch` | **不校验** | 本文 §2.2 |
| query `a_bogus` 在 `/chat/completion` | **已关闭（不校验）**：P1/P2 完全不带 `a_bogus`+`msToken` 仍能建会话出正文；P4 重放旧值也无收益 | [05](doubao2api-05-live-probe-results.md) §1.1 |
| `msToken` | 服务端每响应经 `x-ms-token` 下发，但 query 侧不校验 → **反代可完全忽略** | 05 §1.1、§4 |
| **完整设备 query** | **必需**——砍到 10 键即触发风控滑块（`STREAM_ERROR 710022004`, `type:verify/subtype:slide`）。难的是设备参数自洽，不是签名 | 05 §1.3 |
| 登录态 cookie | 硬门禁，但**可减到 10 个 cookie 名** | 05 §1.2 |

> **§4 的"未决"已由 05 关闭。** 最终形态：签名算法无需实现，反代照抄一份完整设备 query 模板即可。

**修正后的难度评估**：HTTP 头族签名这一关是白送的（省下大量功夫），但 query 侧的
`a_bogus` + `msToken` 在模型端点上**尚未证伪**，仍可能是真关卡。难度上限因此高于 03 之外的乐观估计。

## 5. 下一步（阻塞项只有一个）

需要一次**登录态**抓取：浏览器（或桌面 `Doubao.exe`）挂 mitm 代理 → 登录 → **发一条消息** → 停代理。
这一条能同时关掉 docs/01 §6 的 #1/#3/#4：

1. `messages[]` 元素的真实字段（`messageToSamanthaChatMessage` 插件产物）
2. 首轮 `conversation_id` 为空时服务端是否自动建会话
3. `/samantha/chat/completion` 与 `/samantha/chat/async/stream` 的分工
4. 顺带确认 `chat_ability` 里 `model_type` 数字 ID 的实际传法（见 §6）

## 6. 附带发现：`model_list` 形态

`/alice/user/launch` 的响应里 `data.model_list` 有 **67 项**，每项字段是
`{name, model_type, model_name, typewriter_switch, support_web_search_switch, support_pic_gen_switch}`。

- `model_type` 才是模型 ID：两类值混在一起——Unix 时间戳形态（`1737521813`）和小整数（`1`=ogt、`2`=ogf、`30`、`100`、`113`、`133`…）。
- `name`/`model_name` 是 ASCII 代号（磁盘 hex 已确认 `41 6F 65`=`"Aoe"`，不是编码问题），
  出现最多的代号是 `Aoe`（约 45 项，全部 `model_type` 为时间戳形态），其余为 `ogt`/`ogf`/`ances …`/`func: ogt（16k）` 等。
- 前端把模型选择塞进 `ext.chat_ability`：`createChatAbilityExt(abilityType, JSON.stringify(abilityParams))`
  （`s2-submit-node-generate-message.js` 模块 `IiA`），所以 `chat_ability` 是**字符串化 JSON**。
