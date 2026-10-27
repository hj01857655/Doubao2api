# Doubao2api (Go)

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white) ![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux-lightgrey) ![Type](https://img.shields.io/badge/type-API%20Gateway-blue)

豆包（桌面端 PC Client 上游）→ OpenAI / Anthropic 兼容 API 的 Go 网关，纯 stdlib 零第三方依赖，支持扫码登录。

## 构建 / 运行

```bash
go build -o doubao2api .
./doubao2api          # 默认 127.0.0.1:8378，HOST/PORT 可覆盖
```

- 设了 `DOUBAO_COOKIE` → 数据面直接可用；
- 没设 → 服务照常启动，对话请求回 503；打开 `/login` 扫码，cookie 自动写回 `.env` 并热重建上游客户端，无需重启。

## 凭据来源

两种，二选一：

- **扫码登录**：打开 `http://127.0.0.1:8378/login`（或 `/admin` 面板的醒目入口），用豆包 App 扫码确认，cookie 写入 `DOUBAO_ENV_PATH`（默认状态目录 `.env`），当前进程立即生效。
- **手工配置**：`DOUBAO_COOKIE` 直接给整串 cookie，客户端构造时自动裁到最小必需集（`sessionid` / `sessionid_ss` / `sid_tt` / `sid_guard` / `uid_tt` / `uid_tt_ss` / `ttwid` / `passport_csrf_token`；`d_ticket` / `s_v_web_id` 实测非必需，有则附带）。

## 接口

| 端点 | 说明 |
|---|---|
| `GET /health` | 健康检查（`ok`=凭据就绪、模型列表、上游地址） |
| `GET /v1/models` | 模型列表（`doubao-fast` / `doubao-expert` / `doubao-auto` / `doubao-turbo` / `doubao-pro` / `doubao-2.1-lite`，档位来自本地映射表，不查上游） |
| `POST /v1/chat/completions` | OpenAI Chat Completions（流式 + 会话缓存 + 退避重试） |
| `POST /v1/messages` | Anthropic Messages（Claude Code / Cline 直连） |
| `POST /v1/responses` | OpenAI Responses |
| `GET /login` | 豆包扫码登录页（出码 `POST /login/qr`，轮询 `GET /login/qr/{token}`） |
| `GET /admin` | Web 管理面板（内置，开箱即用）：登录状态、模型档位、上游地址、扫码登录入口。状态目录放一份自定义 `panel.html` 可覆盖内置版 |

`/admin/api/overview` 面板后端：凭据状态 / 模型档位 / 上游 / 是否启用本地鉴权（设了 `API_KEY` 时需带 Bearer 或 `x-api-key`）。

## 配置（环境变量 > `.env` > 默认值）

| 变量 | 默认 | 说明 |
|---|---|---|
| `DOUBAO_COOKIE` | 空 | 上游凭据整串 cookie；缺失时服务照常启动，对话请求 503 |
| `API_KEY` | 空 | 非空 = 入站鉴权（Bearer / x-api-key） |
| `HOST` / `PORT` | `127.0.0.1` / `8378` | 监听地址。Docker 里传 `HOST=0.0.0.0` |
| `DOUBAO_STATE_DIR` | exe 所在目录 | `.env` / `server.log` / `panel.html` 落点 |
| `DOUBAO_ENV_PATH` | 状态目录 `.env` | 扫码登录写回的环境文件路径 |
| `DOUBAO_DEVICE_ID` / `DOUBAO_WEB_ID` / `DOUBAO_PC_VERSION` | 内置值 | 设备面参数，成套覆盖以便多账号轮换（fp=verify_\<device_id\>、tea_uuid=device_id 是强约束） |
| `DOUBAO_BOT_ID` | 内置值 | 主 bot |

## 实现要点（实证结论）

- 不实现任何签名：`a_bogus` / `msToken` 在 `/chat/completion` 端点不被校验
- 设备 query 必须成套自洽，砍参数会触发风控滑块 710022004
- `end_type:1` 即正文完整，立刻发 finish_reason（比流真正结束早 2~3 秒）；`brief` 是会话列表预览串，绝不覆盖拼接正文
- 单账号串行：整条流独占锁，天然限频；上游 710022002（过频）在未吐正文时自动退避重试

## Docker 部署

```bash
cp .env.example .env   # 按需填写
docker compose up -d
```

镜像同时发布到 GHCR：`ghcr.io/hj01857655/doubao2api`（跟随 v* 标签发布，含 latest，linux/amd64 + linux/arm64）。

二进制发版：推送 `v*` 标签后 Actions 自动构建 linux / windows / darwin 的 amd64 + arm64 六平台产物并发布 Release。

> cookie 绑账号不绑机器：`data/` 卷里的 `.env` 持久保存扫码登录结果，重新部署不用再扫码。
