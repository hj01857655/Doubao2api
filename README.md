# Doubao2api

[![python](https://img.shields.io/badge/python-3.10%2B-3776AB?logo=python&logoColor=white)](#快速开始) [![protocols](https://img.shields.io/badge/protocols-OpenAI%20Chat%20%7C%20Anthropic%20Messages%20%7C%20Responses-4B8BBE)](#api-接口) [![runtime](https://img.shields.io/badge/runtime-Flask%20单依赖-000000?logo=flask&logoColor=white)](#快速开始) [![stars](https://img.shields.io/github/stars/hj01857655/Doubao2api?color=yellow)](https://github.com/hj01857655/Doubao2api/stargazers) [![last commit](https://img.shields.io/github/last-commit/hj01857655/Doubao2api)](https://github.com/hj01857655/Doubao2api/commits/main)

> 豆包（Doubao）Web / 桌面端 → OpenAI / Anthropic 兼容 API 网关

把已登录的豆包会话转成本地服务，同时对外提供三种标准协议：OpenAI Chat Completions、Anthropic Messages、OpenAI Responses。**零签名代码**（`a_bogus`/`msToken` 实测不校验），运行期仅依赖 Flask。

本仓库同时收录支撑该网关的**协议逆向取证文档集**（19 篇，入口 [docs/DOUBAO-00-索引.md](docs/DOUBAO-00-索引.md)）：GAC 传输协议、ttnet 加密握手、btrans 帧格式、Saman 更新签名、沙箱运行时等。

> 本项目仅供学习与协议研究。使用真实账号存在风控风险，后果自负；请勿用于生产或转售。

## 功能特性

- **三协议合一**：同一套豆包数据面，三种协议同时出 —— `/v1/chat/completions`、`/v1/messages`、`/v1/responses`
- **流式 + 非流式**：每个协议都支持 `stream: true/false`，流式为标准 SSE 帧（Responses 每帧带 `sequence_number`）
- **无需签名**：不实现任何字节签名算法；cookie 只需 10 个名字
- **多轮会话**：会话历史由豆包服务端持有，客户端回传 `conversation_id` 即可续接
- **入站鉴权**：`Authorization: Bearer` / `x-api-key`（未设 key 则本地裸奔模式）
- **工具上下文可透传**：Anthropic `tool_use` / `tool_result` ⇄ Chat `tool_calls` / `role=tool` 双向翻译完备（但上游不会主动发起调用，见「工具调用：两条不同的面」）
- **轻量**：纯 stdlib + Flask，无浏览器自动化、无 Playwright、无 Redis
- **离线可测**：86 项断言不打真实请求即可验证解析器与协议桥

## 工作原理

上游直连豆包的 chat completion 端点（SSE；桌面端实测路径为 `/chat/completion`，全量契约见 [docs/03](Doubao2api/docs/doubao2api-03-desktop-chat-completion.md)）：

1. 用 10 个必需 cookie 构造会话 + 成套设备 query（**不能砍参数**，砍了触发风控滑块 `710022004`）
2. 把 OpenAI / Anthropic / Responses 请求翻译成豆包五段式信封
3. 解析 SSE 事件流（`STREAM_CHUNK patch_object:1` 为正文权威通道，`CHUNK_DELTA` 不保证出现）
4. 翻译回各协议的标准帧

## 快速开始

### 1. 安装

要求 Python 3.10+。

```bash
pip install -r Doubao2api/requirements.txt   # 只有 flask
```

### 2. 配置

```bash
cp Doubao2api/.env.example Doubao2api/.env
```

编辑 `.env`，填入 `DOUBAO_COOKIE`：登录 [www.doubao.com](https://www.doubao.com/chat/) 后按 F12 → Network → 找 `completion` 请求 → 复制 Cookie 头，保留这 10 个名字即可：

```
sessionid  sessionid_ss  sid_tt  sid_guard  uid_tt  uid_tt_ss
d_ticket  ttwid  s_v_web_id  passport_csrf_token
```

顺手设一个入站 key（可选但建议）：同一个 `.env` 里加一行

```bash
API_KEY=sk-doubao2api-<自选一串>
```

设了之后所有 `/v1/*` 都要求 `Authorization: Bearer <key>` 或 `x-api-key: <key>`（后者适配 Anthropic SDK）；
**`/health` 不校验**，用于探活。不设则入站无鉴权（默认只绑 127.0.0.1）。

### 3. 启动

```bash
python Doubao2api/main.py        # http://127.0.0.1:8378
```

### 4. 验证

```bash
curl http://127.0.0.1:8378/health
```

## API 接口

| 方法 | 路径 | 协议 |
|---|---|---|
| GET | `/health` | 探活 |
| GET | `/v1/models` | 模型列表 |
| POST | `/v1/chat/completions` | OpenAI Chat Completions |
| POST | `/v1/messages` | Anthropic Messages |
| POST | `/v1/responses` | OpenAI Responses |

### 协议翻译架构

```
客户端                     proxy 处理                          上游（豆包）
─────────                 ─────────────                        ─────────────
Chat Completions  ──→  信封构造 + SSE 解析                 ──→  /chat/completion
Responses         ──→  responses_bridge（双向翻译）       ──→  （SSE，五段式信封）
Anthropic         ──→  anthropic_bridge（双向翻译）       ──→
```

三协议共用同一条豆包数据面：Chat 为母语，Responses / Anthropic 是「方言 ⇄ 母语」双向翻译层。

### 模型

`/v1/models` 返回 6 档；`model` 字段传下列任一名字（`doubao` 是 `doubao-fast` 的别名）：

| 模型名 | 说明 |
|---|---|
| `doubao-fast` | 豆包 快速（默认） |
| `doubao-expert` | 豆包 专家 |
| `doubao-auto` | 自动 |
| `doubao-turbo` | 豆包 2.1 Turbo |
| `doubao-pro` | 豆包 2.1 Pro |
| `doubao-2.1-lite` | 豆包 2.1 Lite |

### 调用示例

OpenAI SDK：

```python
from openai import OpenAI

client = OpenAI(api_key="你的 API_KEY", base_url="http://127.0.0.1:8378/v1")
resp = client.chat.completions.create(
    model="doubao-fast",
    messages=[{"role": "user", "content": "你好"}],
    stream=False,
)
print(resp.choices[0].message.content)
```

Anthropic SDK：

```python
from anthropic import Anthropic

client = Anthropic(api_key="你的 API_KEY", base_url="http://127.0.0.1:8378")
msg = client.messages.create(
    model="doubao-fast", max_tokens=1024,
    messages=[{"role": "user", "content": "你好"}],
)
print(msg.content[0].text)
```

curl（流式）：

```bash
curl http://127.0.0.1:8378/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"doubao-fast","stream":true,"messages":[{"role":"user","content":"你好"}]}'
```

### 多轮对话

豆包服务端自己保存会话历史。把上一轮响应里的 `conversation_id`（OpenAI 在 `doubao.conversation_id` 字段，或响应头 `X-Doubao-Conversation-Id`）用请求头带回来：

```bash
curl http://127.0.0.1:8378/v1/chat/completions \
  -H 'X-Doubao-Conversation-Id: <上一轮的 conversation_id>' \
  -H 'Content-Type: application/json' \
  -d '{"messages":[{"role":"user","content":"接着刚才的说"}]}'
```

不带该头时：按请求里的 `user` 字段（Anthropic 为 `metadata.user_id`）在进程内缓存会话映射；新会话且消息数 >1 会把历史压成文本一次带入。

### 鉴权

`.env` 设 `API_KEY`（或 `DOUBAO_API_KEY`）后，请求须携带 `Authorization: Bearer <key>` 或 `x-api-key: <key>`；不设则入站不校验（默认只绑 127.0.0.1）。

## 配置

| 变量 | 必填 | 说明 |
|---|---|---|
| `DOUBAO_COOKIE` | ✅ | 10 个必需 cookie 拼成的字符串，见「快速开始」 |
| `API_KEY` / `DOUBAO_API_KEY` | — | 入站鉴权 key；不设 = 本地裸奔模式 |
| `PORT` | — | 监听端口（默认 `8378`） |
| `DOUBAO_DEVICE_ID` | — | 设备 query 常量（有实测默认值，一般不动） |
| `DOUBAO_WEB_ID` / `DOUBAO_PC_VERSION` / `DOUBAO_CHROMIUM_VERSION` / `DOUBAO_BOT_ID` | — | 同上 |

## 上游指纹与风控

豆包对 chat completion 的身份校验不靠字节签名，靠**成套自洽的设备面**：

| 要件 | 说明 |
|---|---|
| 10 个必需 cookie | 见「配置」；多带无用，**少带会 401 或触发风控** |
| 成套设备 query | `device_id` / `fp` / `tea_uuid` / `web_id` / `web_tab_id` / `runtime` / `pc_version` / `chromium_version` 等必须同源成套 |
| `a_bogus` / `msToken` | 客户端会带，但**服务端实测不校验**，重放旧值亦无收益——这是本项目不需要签名实现的原因 |
| `Authorization` | 上游匿名面，无需本机凭证 |

风控硬闸：**砍设备 query 参数 → 滑块 `710022004 type:verify/subtype:slide`**；过频 → `710022002`。

## 测试

离线测试零真实请求：

```bash
python Doubao2api/tests/test_parser.py        # 52 项：SSE 解析 + 请求面构造
python Doubao2api/tests/test_error_paths.py   #  8 项：错误路径回归（重试不重放正文等）
python Doubao2api/tests/test_bridges.py       # 26 项：三协议桥（含上游实测路径）
```

真实联调纪律：**限频 ≤1 次/分钟、带退避；遇 `710022004`（风控滑块）立即停止自动重试**。

## 文档

### 契约与实现（[Doubao2api/docs/](Doubao2api/docs/)）

| 文档 | 内容 |
|---|---|
| [doubao2api-01](Doubao2api/docs/doubao2api-01-chat-completion-contract.md) | 前端 bundle 静态还原的契约起点 |
| [doubao2api-02](Doubao2api/docs/doubao2api-02-gateway-no-signature.md) | 网关行为与免签名论证 |
| [doubao2api-03](Doubao2api/docs/doubao2api-03-desktop-chat-completion.md) | **权威契约**：端点 / query / 头 / 体 / cookie / SSE 全量定案 |
| [doubao2api-04](Doubao2api/docs/doubao2api-04-sse-response-deepdive.md) | SSE 响应侧实现细节 |
| [doubao2api-05](Doubao2api/docs/doubao2api-05-live-probe-results.md) | 主动探测结论（签名 / cookie 最小集 / 风控阈值） |
| [doubao2api-06](Doubao2api/docs/doubao2api-06-implementation.md) | 实现记录与验证 |

### 客户端逆向集（[docs/](docs/DOUBAO-00-索引.md)，19 篇）

从 [DOUBAO-00 索引](docs/DOUBAO-00-索引.md) 进入，覆盖：应用架构与安装布局、沙箱运行时与 Python 执行栈、**GAC 传输协议全解**、技能云下发体系、UI 层与扩展面、**ttnet 私有模块地图 / 加密握手与运行配置 / btrans body 帧格式**、执行环境组件（沙箱 / 虚拟化 / CUA）、宿主侧模块定档、aha_doctor 环境检测、浏览器扩展面、sscronet 导出面、剩余原生域、ttnet 加密帧层深挖、未深挖与后续。

> 逆向**证据**（工具链、字符串转储、native-network 产物、抓包样本）保留在本地 `workspace/` 与 `tools/samples/`，**不入库**（体积与环境相关）；文档中指向它们的路径是本地路径，克隆后不存在。`tools/` 下入库的是脚本与清单：`reqable_mdb.py`（只读解析 Reqable 抓包库）、`reqable_inventory.md`（332 会话去重清单）、`chunkurl.py` / `mitm_addon.py` / `run_desktop.ps1`。

## 工具调用：两条不同的面

**本代理面**（Web `/chat/completion`）没有 OpenAI 式 function calling：请求信封固定五键
`client_meta` / `messages` / `option` / `user_context` / `ext`，**没有 `tools` 声明位**；响应 SSE 也没有
`tool_calls` 帧。桥层能做的是**工具上下文的翻译与透传**——Anthropic 的 `tool_use` / `tool_result`
会翻成 `tool_calls` / `role=tool` 随请求带上——但模型不会据此主动发起调用。

**豆包自己的工具能力在另一条面上**（不在本代理范围）：技能云 / 技能商店 / 连接器 / CUA 任务模式。
端点清单里能直接看到这条面：

| 端点 | 含义 |
|---|---|
| `/samantha/tool/upload_tool_call_result` | 客户端把工具调用结果回传上游 |
| `/alice/office/tool_local/upload_tool_call_result(_v2)` | 同上（办公场景） |
| `/alice/office/skills/manage/*` | 技能安装 / 详情 / 启停 / 商店 / 插件 / 连接器 OAuth |
| `/alice/bot/skill_cards` | 技能卡片 |

机制细节见 [docs/04 技能云下发体系](docs/DOUBAO-04-技能云下发体系.md)、
[docs/09 沙箱虚拟化与 CUA](docs/DOUBAO-09-执行环境组件-沙箱虚拟化与CUA.md) 与
[DOUBAO-TASK-MODE-AUDIT](docs/DOUBAO-TASK-MODE-AUDIT.md)：模型只在沙箱里生成动作，经 GAC 协议
（ChaCha20-Poly1305 帧 + 三凭证会话）送回宿主执行，**模型代码碰不到 Win32**。

## 已知限制

- **本通道无 function calling**：请求信封没有 `tools` 声明位、响应无 `tool_calls` 帧，模型不会按你给的 schema 主动调用（豆包的工具能力在技能面，见上）
- **图片丢弃**：请求里的图片块在数据面被丢弃，不透传
- **无 reasoning**：上游 SSE 无思考通道，Responses 的 reasoning item 不会出现
- **usage 为估算**：豆包不回 token 数，usage 按字符数近似
- **风控是真实风险**：设备 query 必须成套自洽；账号处于被观察状态时请停手

## 免责声明

- 本项目仅用于学习研究 HTTP/SSE 协议、二进制传输栈与 API 网关设计，**协议验证 ≠ 生产使用**。
- 抓包样本一律脱敏（头名与哈希，值不落盘）；**凭据文件不读取、不入库**。
- 使用本项目产生的账号风控后果由使用者自行承担；请遵守豆包服务条款。
