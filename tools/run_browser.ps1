#!/usr/bin/env pwsh
# Phase 0 · 启动抓包专用浏览器实例（与桌面端 Doubao.exe 完全隔离，不影响正在进行的 frida）
# 用法：pwsh -File tools/run_browser.ps1
$ErrorActionPreference = 'Stop'

$profile = Join-Path $env:TEMP 'doubao-mitm-edge'
if (-not (Test-Path -LiteralPath $profile)) { New-Item -ItemType Directory -Path $profile | Out-Null }

# 系统信任库里是否已有 mitmproxy CA（首跑需要装一次，见 README Phase 0 证书步骤）
$thumb = (Get-ChildItem Cert:\CurrentUser\Root -ErrorAction SilentlyContinue |
    Where-Object Subject -like '*mitmproxy*' | Select-Object -First 1).Thumbprint
if ($thumb) {
    Write-Output "mitmproxy CA 已在 CurrentUser\Root（$($thumb.Substring(0,12))…），可直接抓包"
} else {
    Write-Output "⚠ 未安装 mitmproxy CA 到 CurrentUser\Root —— Edge 将报证书错误。"
    Write-Output "  装法（无需管理员，且不动系统级）：certutil -addstore -user Root %USERPROFILE%\.mitmproxy\mitmproxy-ca-cert.cer"
    Write-Output "  （本脚本不代替你执行这条改动信任库的命令，由你决定）"
}

$edge = "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
Write-Output "启动 Edge（独立 profile + 仅本实例代理，不碰系统代理设置）"
Start-Process -FilePath $edge -ArgumentList @(
    '--proxy-server=http://127.0.0.1:8080',
    "--user-data-dir=`"$profile`"",
    '--no-first-run',
    '--no-default-browser-check',
    'https://www.doubao.com/'
)
