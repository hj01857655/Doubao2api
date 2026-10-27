# Doubao2api 凭据

- 登录：`/login` 扫码，凭据落 `<状态目录>/cookie.json`（0600，含 saved_at）。
- 兼容：`.env` 的 `DOUBAO_COOKIE` 手工粘贴仍可读取（未设置 env 时优先 cookie.json）。
- 上游：`https://www.doubao.com/chat/completion`（cookie 会话）。
- 安全：cookie 等同会话凭证，`cookie.json`/`.env` 均不入库（.gitignore 已挡）；曾发生过
  一次 .env 连同 cookie 误提交公开仓库的事故，已重建历史清除——cookie 建议定期轮换。
