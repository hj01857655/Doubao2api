# Doubao2api (Go)

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white) ![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux-lightgrey) ![Type](https://img.shields.io/badge/type-API%20Gateway-blue)

豆包转 OpenAI / Anthropic 兼容 API 的 Go 网关实现，支持扫码登录。

## 构建

```bash
go build -o doubao2api .
```

## 运行

启动前必须配置登录凭据（Cookie 或扫码登录）：

| 环境变量 | 说明 |
|---|---|
| `DOUBAO_COOKIE` | 登录 Cookie（与扫码登录二选一） |
| `DOUBAO_ENV_PATH` | 环境文件路径 |
| `PORT` | 监听端口，默认 `8378` |
| `DOUBAO_STATE_DIR` | 状态目录 |

## 接口

- `GET /health` — 健康检查
- `GET /v1/models` — 模型列表
- `POST /v1/chat/completions` — OpenAI Chat 兼容
- `POST /v1/messages` — Anthropic Messages 兼容
- `POST /v1/responses` — OpenAI Responses 兼容
- `GET /login`、`GET /login/qr` — 扫码登录

## Docker 部署

```bash
# 配置环境变量后启动
cp .env.example .env   # 按需填写
docker compose up -d
```

镜像同时发布到 GHCR：`ghcr.io/hj01857655/hj01857655/Doubao2api`（跟随 v* 标签发布，含 latest，linux/amd64 + linux/arm64）。

二进制发版：推送 `v*` 标签后 Actions 自动构建 linux / windows / darwin 的 amd64 + arm64 六平台产物并发布 Release。
