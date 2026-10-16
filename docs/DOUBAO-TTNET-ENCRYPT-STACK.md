# ttnet 加密栈（已拆分为主题文档）

本稿内容已按主题拆分进文档集，本文件保留为**索引存根**（原路径引用仍有效）：

| 原章节 | 现位置 |
|---|---|
| §1 模块面与工具边界、§2 私有模块地图（bytetransport/frontierx/encrypt 等） | [06 ttnet 私有模块地图](DOUBAO-06-ttnet私有模块地图.md) |
| §3 `encrypt_config_v1`（bk_k/bk_t/字段语义）、§3.3 ttnet 全配置面、§4 加密头族、§6 握手入口结论 | [07 ttnet 加密握手与运行配置](DOUBAO-07-ttnet加密握手与运行配置.md) |
| §5 body 帧格式（帧类型/字段/约束/错误码） | [08 btrans body 帧格式](DOUBAO-08-btrans-body帧格式.md) |
| §7 剩余工作 | [99 未深挖与后续](DOUBAO-99-未深挖与后续.md) |
| §8 产物索引 | [00 索引](DOUBAO-00-索引.md) + `workspace/native-network/` |

> 关键纠正（随拆分一并固化）：配置值的正确来源是 **doubao 自身** `User Data/ahanet/server.json`，
> 而非 DoubaoWork 2.31 的内存转储（那是另一个产品）——详见 07 §5。