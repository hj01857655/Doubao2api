# Doubao2api

豆包网页版 → OpenAI / Anthropic 兼容 API 网关。

把已登录的豆包 Web 会话转成本地服务，同时对外提供三种标准协议：OpenAI Chat Completions、Anthropic Messages、OpenAI Responses。**零签名代码**（`a_bogus`/`msToken` 实测不校验），运行期仅依赖 Flask。

> 本项目仅供学习与协议研究。使用真实账号存在风控风险，后果自负；请勿用于生产或转售。

## 特性

- **三协议合一**：同一套豆包数据面，三种协议同时出——`/v1/chat/completions`、`/v1/messages`、`/v1/responses`
- **流式 + 非流式**：每个协议都支持 `stream: true/false`，流式为标准 SSE 帧（Responses 每帧带 `sequence_number`）
- **无需签名**：不实现任何字节签名算法；cookie 只需 10 个名字
- **多轮会话**：会话历史由豆包服务端持有，客户端回传 `conversation_id` 即可续接
- **入站鉴权**：`Authorization: Bearer` / `x-api-key`（未设 key 则本地裸奔模式）
- **轻量**：纯 stdlib + Flask，无浏览器自动化、无 Playwright、无 Redis
- **离线可测**：86 项断言不打真实请求即可验证解析器与协议桥

## 工作原理

上游直连豆包 Web 的 `POST /samantha/chat/completion`（SSE）：

1. 用 10 个必需 cookie 构造会话 + 成套设备 query（**不能砍参数**，砍了触发风控滑块 `710022004`）
2. 把 OpenAI/Anthropic/Responses 请求翻译成豆包五段式信封
3. 解析 SSE 事件流（`STREAM_CHUNK patch_object:1` 为正文权威通道，`CHUNK_DELTA` 不保证出现）
4. 翻译回各协议的标准帧

契约逆向过程与证据链见 [docs/](docs/)（01 前端 bundle 静态还原 → 03 端点全量定案 → 05 主动探测 → 06 实现）。

## 快速开始

### 1. 安装

要求 Python 3.10+。

```bash
pip install -r requirements.txt   # 只有 flask
```

### 2. 配置

```bash
cp .env.example .env
```

编辑 `.env`，填入 `DOUBAO_COOKIE`：登录 [www.doubao.com](https://www.doubao.com/chat/) 后按 F12 → Network → 找 `completion` 请求 → 复制 Cookie 头，保留这 10 个名字即可：

```
sessionid  sessionid_ss  sid_tt  sid_guard  uid_tt  uid_tt_ss
d_ticket  ttwid  s_v_web_id  passport_csrf_token
```

### 3. 启动

```bash
python main.py        # http://127.0.0.1:8378
```

### 4. 验证

```bash
curl http://127.0.0.1:8378/health
```

## 使用

### 端点

| 方法 | 路径 | 协议 |
|---|---|---|
| GET | `/health` | 探活 |
| GET | `/v1/models` | 模型列表 |
| POST | `/v1/chat/completions` | OpenAI Chat Completions |
| POST | `/v1/messages` | Anthropic Messages |
| POST | `/v1/responses` | OpenAI Responses |

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

## 配置项

| 变量 | 必填 | 说明 |
|---|---|---|
| `DOUBAO_COOKIE` | ✅ | 10 个必需 cookie 拼成的字符串 |
| `API_KEY` / `DOUBAO_API_KEY` | — | 入站鉴权 key；不设 = 本地裸奔模式 |
| `PORT` | — | 监听端口（默认 `8378`） |
| `DOUBAO_DEVICE_ID` | — | 设备 query 常量（有实测默认值，一般不动） |
| `DOUBAO_WEB_ID` / `DOUBAO_PC_VERSION` / `DOUBAO_CHROMIUM_VERSION` / `DOUBAO_BOT_ID` | — | 同上 |

## 已知限制

- **工具调用不闭环**：上游豆包无 function calling；桥层的 tool_use/tool_calls 翻译是完备的，但模型不会主动发起（antml 文本回收器已就绪未接线）
- **图片丢弃**：请求里的图片块在数据面被丢弃，不透传
- **无 reasoning**：上游 SSE 无思考通道，Responses 的 reasoning item 不会出现
- **usage 为估算**：豆包不回 token 数，usage 按字符数近似

## 测试

离线测试零真实请求：

```bash
python tests/test_parser.py        # 52 项：SSE 解析 + 请求面构造
python tests/test_error_paths.py   #  8 项：错误路径回归（重试不重放正文等）
python tests/test_bridges.py       # 26 项：三协议桥（含上游实测路径）
```

真实联调纪律：**限频 ≤1 次/分钟、带退避；遇 `710022004`（风控滑块）立即停止自动重试**。

## Roadmap

- [x] Phase 0/1 · MITM 取证 + `/chat/completion` 全契约还原（docs/03）
- [x] Phase 3 · 三协议壳 + 入站鉴权 + 目录分层
- [ ] Phase 2 · 鉴权复现：cookie 过期行为、`x-ms-token` 轮转、设备 query 最小集
- [ ] 多账号 Cookie 轮询（参考社区同类实现）
- [ ] Docker 部署
- [ ] （可选）CLIProxyAPI Go 插件形态

## 文档

| 文档 | 内容 |
|---|---|
| [docs/doubao2api-01](docs/doubao2api-01-chat-completion-contract.md) | 前端 bundle 静态还原的契约起点 |
| [docs/doubao2api-02](docs/doubao2api-02-gateway-no-signature.md) | 网关行为与免签名论证 |
| [docs/doubao2api-03](docs/doubao2api-03-desktop-chat-completion.md) | **权威契约**：端点/query/头/体/cookie/SSE 全量定案 |
| [docs/doubao2api-04](docs/doubao2api-04-sse-response-deepdive.md) | SSE 响应侧实现细节 |
| [docs/doubao2api-05](docs/doubao2api-05-live-probe-results.md) | 主动探测结论（签名/cookie 最小集/风控阈值） |
| [docs/doubao2api-06](docs/doubao2api-06-implementation.md) | 实现记录与验证 |

## 免责声明

- 本项目仅用于学习研究 HTTP/SSE 协议与 API 网关设计，**协议验证 ≠ 生产使用**。
- 抓包样本一律脱敏（头名与哈希，值不落盘）；凭据文件不读取、不入库。
- 使用本项目产生的账号风控后果由使用者自行承担；请遵守豆包服务条款。
