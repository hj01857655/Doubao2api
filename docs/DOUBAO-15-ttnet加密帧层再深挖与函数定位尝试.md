# 15 ttnet 加密/帧层函数级反汇编

主题：把 [07 加密握手](DOUBAO-07-ttnet加密握手与运行配置.md) / [08 btrans 帧](DOUBAO-08-btrans-body帧格式.md) 推进到**函数级**——
加密握手函数群、密钥调度函数与 btrans 处理函数的定位 + 反汇编。
方法：功能串 xref 定位（§1）+ `capstone` 定向/全量反汇编；脚本 `workspace/tools/{xref_diag,locate_encrypt,disasm_fn,disasm_full,locate_more}.py`。

## 1. 方法：`.cc` 锚点不可用，**功能串 xref 可用**

`.cc` 锚点（`tt_encrypt_key_manager.cc` / `btrans_framer.cc`）**零代码引用**——先用**对照组**验证扫描器无误
（`Masked frame from server`×2、`NOT RESUMABLE`×1、`heartbeat`×5 均有 xref），说明它们位于只读日志数据集、**间接引用**。
**进路**：改**顺邻域功能串的 xref**（`encrypt_config_v1`/`secp256r1`/`c hs traffic` 等被代码直接引用），其 xref 所在函数即处理函数。

## 2. ttnet 加密函数群（已定位 + 反汇编）

| 函数 RVA | 长度 | 引用串 | 定性 |
|---|---|---|---|
| `0x27dff0` | 2702 | `encrypt_config_v1` + 全部配置键（`bk_k`/`bk_t`/`url`/`enabled`/`ver`/各类阈值比值）+ 常量 `504273700a3d721f` | **`encrypt_config_v1` 解析器** |
| `0x284890` | 1305 | `version`/`random`/`app_id`/`did`/`curve`/`secp256r1`/`pubkey`/`key_shares`/`cipher_suites` | **握手首飞（ClientHello）构造器** |
| `0x284eb0` | 4120 | `POST`/`OnURLFetchComplete`/`x-tt-s-sign`/`x-tt-s-cert-sign`/`cert`/`key_share`/`pubkey`/`cipher_suite`/`ticket`/`ticket_exp`/`ticket_long(_exp)` + `tt_encrypt_key_manager.cc` + 常量 `6a15c5844fffc436` | **keyhub 握手响应处理器** |
| `0x284db0` | 254 | 取 `[rdi+0xc8]` 的 **32 字节密钥**（`edx=0x20`）→ `0x45a840` | **ECDSA 运算**（见 §3） |
| `0x27eb10` | 2018 | `ee.info` | EncryptedExtensions 处理器 |
| `0x44ca30` | 210 | `NOT RESUMABLE` | 会话复用检查 |

## 3. 加密原语 = 静态链接的 boringssl（已确认）

加密函数调用的 `0x459xxx–0x45bxxx` 簇中：
- `0x45b300` 引用 `..\..\third_party\boringssl\src\crypto\bio\bio.c`
- `0x45a420` 引用 `..\..\third_party\boringssl\src\crypto\ecdsa_extra\ecdsa_asn1.c` → **ECDSA（签名/验签）**（链路 `0x284db0 → 0x45a840 → 0x45a420`）

→ **该簇是静态链接进 sscronet 的 boringssl**；boringssl 自身 `hkdf.c`/`ecdh_extra.c`/`ecdsa_asn1.c` 的 `.c` 串均为**断言串（0 xref）**，故经其 `.c` 串定位不到函数，但"加密走 boringssl"已由 `bio.c`/`ecdsa_asn1.c` 引用证实。

## 4. 函数级密钥调度（TLS 1.3，RFC 8446 §7.1）

**全部 TLS 1.3 key-schedule label 在二进制中命中**（各 ×1）：

```
c hs traffic   s hs traffic   c ap traffic   s ap traffic
derived        exp master     res master     res binder
client finished   server finished
```
（另 `tls13` ×8、`hkdf`/`HKDF` ×3、`tls13_enc` ×1）

label 的 xref 精确落到密钥调度各步函数：

| 函数 RVA | 承载 label | 定性 |
|---|---|---|
| `0x6b0a60` | — | **`HKDF-Expand-Label`**（核心原语） |
| `0x6b11b0` | `c hs traffic` / `s hs traffic` | 握手流量密钥 |
| `0x6b1410` | `c ap traffic` / `s ap traffic` / `exp master` | 应用流量密钥 |
| `0x6b0530` | `client finished` / `server finished` | Finished MAC 密钥 |
| `0x6b18b0` | `res master` | 复用主密钥 |
| `0x6b2040` | `res binder` | 复用 binder（PSK） |

**反汇编 `0x6b11b0` 实锤**：

```asm
006b1266  lea  rax, [rip+0x1df984]   ; "c hs traffic"     ← label 立即数
006b1272  mov  [rsp+0x70], 0xc       ; 0xc = 12 = label 长度
006b12bb  call 0x1806b0a60           ; → HKDF-Expand-Label
006b12d6  lea  rdx, [rip+0x1df921]   ; "CLIENT_HANDSHAKE_TRAFFIC_SECRET"（NSS keylog 格式）
006b12e5  call 0x1803f7d50           ; → keylog 回调
```

**结论**：这套 TLS 1.3 密钥调度属于 sscronet 的 **TLS 层**（豆包 HTTPS 走它）。frida 活抓（`workspace/native-network/frida_kdf_capture.md`）：

- `HKDF-Expand-Label` = `0x6b0a60`；`info` = `"tls13 " + label`；label = RFC 8446 标准值（hex 坐实）。
- 该层握手 PSK 为空：Early Secret = `HKDF-Extract(0,0)` = `33ad0a1c…170f92a`。
- `HKDF-Extract(0, bk_k)` = `e84502db…` ≠ Early Secret → `bk_k` 不是 TLS 层的 PSK。

`encrypt_config_v1` 的加密通道是**另一层**：keyhub 自定义握手（`0x284890` 首飞构造 / `0x284eb0` 响应处理），
会话密钥 = ECDH(`secp256r1`) 共享密钥 → ChaCha20（协议见 [07 §7](DOUBAO-07-ttnet加密握手与运行配置.md)）。

### 4.1 `bk_k` / `bk_t` 的落点（配置结构布局，实测）

`encrypt_config_v1` 解析器（`0x27dff0`）把配置写进一个带 **vtable `0x7f43b0`** 的对象：

| 字段 | 结构偏移 | 类型 |
|---|---|---|
| vtable | `+0x00` | `0x7f43b0` |
| `bk_k`（base64 解码后） | `+0x38` | std::string |
| `bk_t`（基钥票据） | `+0x50` | std::string |

该对象的 vtable 被 **握手响应处理器 `0x284eb0`**（vtable xref 之一）引用；其 vtable 方法群在 `0x287650` / `0x2885a0` / `0x288600` / `0x288640` / `0x288680`。
`bk_k`/`bk_t` 即 keyhub 握手（`0x284890`/`0x284eb0`）的会话密钥材料（ECDH → ChaCha20，见 07 §7）。

## 5. btrans 处理函数与帧结构

| 函数 RVA | 引用串 | 定性 |
|---|---|---|
| `0x478ad0` | `btrans_dict`/`racing_strategy`/`tls_race_delay_ms`/`fast_ip_timeout_ms`/`send_block_bytes`/`max_queued_write_bytes`/`disable_quic_broken` | btrans 配置装载 |
| `0x485180` | `heartbeat_max_failures` | btrans 心跳 |
| `0x47a8c0` | `RequestSession`/`session_key:` | btrans 会话请求 |
| `0x487fd0` | `frame_type=` / `, frame_flags=` / `, stream_id=` / `, public_payload_length=` | **帧头 toString**（帧头字段） |
| `0x47bfe0` | `, is_ack: ` / `, is_reset: ` / `, error_code: ` | 控制帧 toString（ack/reset/error） |
| `0x484330` | `, weight=` / `, end_message: ` / `, need_ack: ` / `, padding_length=` / `, meta_size=` / `, content_size=` | 消息帧 toString（消息体字段） |
| `0x488350` | （第三个帧 toString 调用者） | 帧处理链成员 |

**帧结构字段（由帧 toString 函数实锤）**——补 [08](DOUBAO-08-btrans-body帧格式.md)：

| 字段 | 结构偏移 | 宽度 |
|---|---|---|
| `public_payload_length` | `+0x08` | qword |
| `frame_type` | `+0x10` | byte |
| `frame_flags` | `+0x18` | qword |
| `stream_id` | `+0x20` | qword |
| `is_ack` / `is_reset` / `error_code` | — | 控制帧字段 |
| `weight` / `end_message` / `need_ack` / `padding_length` / `meta_size` / `content_size` | — | 消息帧字段 |

> `frame_type` 的 toString（`0x487fd0`）有 3 个调用者：`0x47bfe0` / `0x484330` / `0x488350`。
> 帧的**线格式字节编码**（字节序/宽度/字段偏移的序列化）对应串 `frame_flags`/`public_payload_length`/`frame_header`/`header_size` 均 0 xref（同 `.cc` 锚点，只读数据集）。
> [08](DOUBAO-08-btrans-body帧格式.md) + 本节字段表为帧格式的静态结论。

## 6. 结论

- sscronet 的 **TLS 层** = boringssl TLS 1.3（frida 活抓）：`HKDF-Expand-Label`=`0x6b0a60`，label = RFC 8446 标准值；该层 PSK 为空，`bk_k` 不是它的 PSK。
- ttnet **加密通道**（`bk_k`/`bk_t`）= keyhub 自定义握手（`0x284890`/`0x284eb0`）：ECDH(`secp256r1`) → ChaCha20 会话加密（协议见 07 §7）。
- **加密函数群 + btrans 处理函数**已定位反汇编；**帧结构字段**已取。
- 帧的**线格式字节编码**（字节序/宽度/偏移）未取到实现体。
- 工具链：`pefile` + `capstone` 定点分析，不需要 rea，无 330s 超时。

## 7. 动态取证方案（frida，已就绪）

帧线格式、keyhub 握手的密钥材料用 **frida 动态** 取（本机 frida 17.19），脚本落盘：

| 文件 | 作用 |
|---|---|
| `workspace/tools/frida_ttnet.js` | 挂 `sscronet.dll` hook：`0x6b0a60`（HKDF-Expand-Label，dump **secret/label/context**）+ key-schedule 各步（`0x6b11b0`/`0x6b1410`/`0x6b0530`/`0x6b18b0`/`0x6b2040`）+ `0x284eb0`(keyhub 响应) + `0x27dff0`(配置解析) + 帧 toString（`0x487fd0`/`0x47bfe0`/`0x484330`，dump 帧对象） |
| `workspace/tools/frida_run.py` | attach runner：枚举 / 按名 / 按 pid 挂到豆包，加载上面的 JS 并打印 hook 输出 |

用法（**豆包需在跑**）：

```bash
python workspace/tools/frida_run.py            # 自动找名字含 doubao 的进程
python workspace/tools/frida_run.py Doubao.exe # 按进程名
python workspace/tools/frida_run.py <pid>      # 按 pid
```

**产出**：豆包联网后 dump `HKDF-Expand-Label` 的 secret/label/context 与帧对象字段值。
若 attach 失败（进程受保护/提权）→ 以**管理员身份**运行脚本。