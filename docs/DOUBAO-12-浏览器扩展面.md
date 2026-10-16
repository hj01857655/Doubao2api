# 12 浏览器扩展面：ai-views / rpa-component-extension

主题：`app/local_webcontents/extensions/` 下两个 Chrome 扩展——AI 侧边栏注入与浏览器自动化。
证据：两份 `manifest.json` 实测 + 扩展目录结构。

## 1. 扩展清单

| 扩展 | 版本 | 名称 | 定性 |
|---|---|---|---|
| `ai-views` | 1.0.4.9033 | `__MSG_name__`（i18n，zh_CN） | **AI 侧边栏注入**——在所有网页注入 AI 助手 UI |
| `rpa-component-extension` | 0.21.30 | 豆包浏览器自动化助手 | **浏览器自动化**——CUA"操作浏览器"能力的扩展侧 |

## 2. `ai-views`——AI 侧边栏注入

### 2.1 Manifest 要点（MV3）

| 项 | 值 |
|---|---|
| Background | `static/js/background.js`（module 类型 service worker） |
| Side panel | `side_panel.html`（Chrome 侧边栏 API） |
| host_permissions | `<all_urls>` |
| Options | `options.html`（open_in_tab） |

### 2.2 权限面

```
storage, cookies, tabs, webRequest, sidePanel, scripting, contextMenus,
declarativeNetRequest, declarativeNetRequestWithHostAccess,
declarativeNetRequestFeedback, webNavigation, bookmarks
```

> `declarativeNetRequest` 全三档 + `webRequest` + `cookies` + `bookmarks`——可**拦截/改写网络请求、读写 Cookie、访问书签**。

### 2.3 Content Scripts（三组）

| 匹配 | 注入时机 | 脚本 |
|---|---|---|
| `<all_urls>`（排除 doubao/cici/ciciai/dola 的 `/chat/**` 和 `/cross-site-support`） | `document_end` | 14 个 chunked JS + `content.js` |
| `<all_urls>` | `document_start` | `preinject.js`（最早注入，抢在页面脚本前） |
| `doubao/cici/ciciai/dola` 的 `/chat/**` | `document_start` | `homepage_scripts.js` |

> 域名族：`doubao.com` / `cici.com` / `ciciai.com` / `dola.com`——豆包的多品牌域名。排除 `/chat/**` 是因为聊天页自身已内置 AI，无需再注入侧边栏。

## 3. `rpa-component-extension`——浏览器自动化助手

### 3.1 Manifest 要点（MV3）

| 项 | 值 |
|---|---|
| 名称 | 豆包浏览器自动化助手 |
| 描述 | "在你的允许下，让智能助手使用豆包内置浏览器完成自动化任务。" |
| Background | `background.js`（service worker） |
| Action | `popup.html` |
| 公钥 | 固定（稳定扩展 ID） |

### 3.2 权限面

```
debugger, tabs, tabGroups, activeTab, alarms, storage, unlimitedStorage,
nativeMessaging, downloads
```

| 权限 | 能力 |
|---|---|
| **`debugger`** | Chrome DevTools Protocol 全量访问——DOM 检查、JS 执行、网络拦截、截图、输入模拟 |
| `tabs` / `tabGroups` / `activeTab` | 标签页管理 |
| `nativeMessaging` | 与原生宿主（豆包主程序）双向通信 |
| `downloads` | 下载管理 |
| `storage` / `unlimitedStorage` | 持久存储 |
| `alarms` | 定时任务 |

### 3.3 host_permissions

```
<all_urls>, http://127.0.0.1/*
```

> `127.0.0.1` 权限——与本地服务通信（可能是 agent_infra 或内置代理）。

### 3.4 无 content_scripts

纯 background + popup 架构：所有页面操作经 `debugger` API（CDP）远程完成，不注入 content script。

## 4. 与 CUA 链路的关系

```
模型决策"操作浏览器"
  → agent_infra（10，cua:call_tool）
    → aha_cua.dll（09，桌面自动化）
      ↘ 但浏览器内操作走另一路：
        → rpa-component-extension（本篇，debugger/CDP）
          → Chrome 内目标页面（DOM/网络/输入）
```

即：**桌面级操作**走 `aha_cua`（Win32 GUI 自动化），**浏览器内操作**走 `rpa-component-extension`（CDP 精确自动化）。两条路在 `agent_infra` 层汇合，由 `cua:call_tool` 的 `method` 字段分派。

`nativeMessaging` 通道是 rpa 扩展与豆包主程序之间的桥——扩展收到主程序的自动化指令后，用 `debugger` API 在 Chrome 内执行。

## 5. `ai-views` 与 `rpa-component-extension` 的分工

| | ai-views | rpa-component-extension |
|---|---|---|
| 注入方式 | content script（注入到页面） | debugger API（远程操控） |
| 主要能力 | UI 注入（侧边栏）、请求拦截、Cookie/书签访问 | 页面自动化（DOM/JS/网络/输入/截图） |
| 用户可见 | 侧边栏 UI | popup + 自动化执行（用户"允许下"） |
| 触发 | 页面加载自动注入 | 主程序经 nativeMessaging 下发指令 |

## 6. 未挖

- `ai-views` 的 14 个 content script chunk 的**具体注入逻辑**（侧边栏 UI 构造、请求改写规则）。
- `rpa-component-extension` 的 `background.js` 中 **nativeMessaging 协议格式**（指令 schema）。
- `preinject.js` 在 `document_start` 注入的**抢跑逻辑**（是否 hook 全局对象）。
- `options.html` 的配置项。

## 7. 证据与产物

| 产物 | 位置 |
|---|---|
| Manifest 全文 | `workspace/extensions/manifests.txt` |