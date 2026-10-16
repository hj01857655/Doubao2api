#!/usr/bin/env pwsh
# 桌面端 Doubao.exe 走 mitmproxy（与浏览器路线互补：桌面端自带登录态，免扫码）
#
# 已知风险（DOUBAO-06/07/13）：模型流量可能走 sscronet（cronet fork + BoringSSL 内置根），
# 不吃 --proxy-server 或不认系统 CA。判读方法：跑完后看 capture.log 里
#   - 出现 www.doubao.com 之外的 doubao 域名 + POST/长连接  => 桌面路线可用
#   - 只有 accounts/passport/静态资源，无业务面            => 模型流量绕过代理
#
# 用法：pwsh -File tools/run_desktop.ps1        （需要 mitmproxy 已在 8080 监听）
param(
    [string]$ProxyServer = 'http://127.0.0.1:8080',
    # 复用真实 profile = 有登录态；代价是可能与已运行实例冲突（实测见下方提示）
    [switch]$UseRealProfile
)
$ErrorActionPreference = 'Stop'

$exe = "$env:LOCALAPPDATA\Doubao\Application\app\Doubao.exe"
if (-not (Test-Path -LiteralPath $exe)) {
    $exe = "$env:LOCALAPPDATA\Doubao\Application\Doubao.exe"
}
if (-not (Test-Path -LiteralPath $exe)) { "未找到 Doubao.exe"; exit 1 }

$running = @(Get-CimInstance Win32_Process -Filter "Name='Doubao.exe'" |
    Where-Object { $_.CommandLine -notmatch '--type=' })
if ($running.Count -gt 0) {
    Write-Output "⚠ 桌面端已在运行（主进程 PID $($running[0].ProcessId)）。"
    Write-Output "  新实例大概率被单实例锁接管、并沿用旧实例的网络栈 —— 代理参数会失效。"
    Write-Output "  建议先退出托盘里的豆包，再跑本脚本。"
}

$listen = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
if (-not $listen) { Write-Output "⚠ 8080 无监听 —— 先跑： .\.venv\Scripts\python.exe tools\mitm_addon.py"; exit 1 }

$profile = Join-Path $env:TEMP 'doubao-mitm-profile'
if (-not (Test-Path -LiteralPath $profile)) { New-Item -ItemType Directory -Path $profile | Out-Null }

# CA 已装进 CurrentUser\Root（实测 4DDE293819DE…），无需任何忽略证书开关
$chromiumArgs = @("--proxy-server=$ProxyServer", '--no-first-run')
if ($UseRealProfile) {
    Write-Output "使用真实 profile（含登录态，会读写你的账户数据目录）"
} else {
    Write-Output "使用隔离 profile：$profile （**无登录态**，需要你在窗口里登录一次）"
    $chromiumArgs += "--user-data-dir=$profile"
}

Start-Process -FilePath $exe -ArgumentList $chromiumArgs
Write-Output "已启动，代理 = $ProxyServer。发一条消息后看 tools/capture.log 的 NEW_HOST 与 WS 行。"
