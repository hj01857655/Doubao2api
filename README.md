# Doubao

豆包桌面端（Windows）逆向工程：静态分析、动态验证、协议还原的完整工作区。

## 内容

- **docs/** — 19 篇逆向文档（DOUBAO-00 索引起步）：ttnet 加密握手、btrans 帧格式、Saman 更新签名、aha_doctor 硬件指纹、keyhub 握手协议（ECDH secp256r1 → ChaCha20）、frida 动态验证记录等。入口见 [docs/DOUBAO-00-索引.md](docs/DOUBAO-00-索引.md)。
- **workspace/tools/** — 逆向工具链：PE 解析与函数定位（pe_recon / xref_scan / disasm_fn / callers 等，pefile+capstone 定点分析路线）、frida 动态验证脚本（frida_ttnet.js / frida_spawn.py）。
- **workspace/native-network/** — sscronet.dll 导出面（1381 导出）、frida 密钥调度抓取证据、ttnet 网络层证据。
- **workspace/modules/ / aha_doctor/ / sbox-vm/ / extensions/** — 各原生模块的字符串转储与分析产物。
- **Doubao2api/** — 独立子项目：豆包 Web → OpenAI/Anthropic 兼容 API 网关（另见其仓库与 README）。

## 方法论要点

- 大体量原生 DLL（sscronet.dll 9.86MB）走 **strings 层证据 + pefile/capstone 定点反汇编**（Ghidra/rea 全量分析会超时）；功能串 xref 优于 `.cc` 锚点定位。
- 静态到顶后转 **frida 动态验证**（spawn + 紧密轮询 attach，早挂配方见 frida_spawn.py）；密钥调度以 RFC 8446 标准值逐字节坐实。
- 协议层结论与社区公开文档（doubaoime-asr 的 wave_protocol.md）交叉互证。

## 纪律

- 账户/token 存储刻意不碰（隐私）；抓包证据一律脱敏。
- 本仓库仅用于学习研究，不提供任何绕过风控的工具。
