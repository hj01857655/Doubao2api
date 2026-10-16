# 07 ttnet 加密握手与运行配置

主题：豆包把请求加密送出去的**全部配置面**——握手入口、基钥/票据、加密头族、调度与隧道策略。
证据：`C:\Users\12925\AppData\Local\Doubao\User Data\ahanet\server.json`（12,585 字节明文，`data` 共 30 项）
+ `app/sscronet.dll` strings。证据副本：`workspace/native-network/doubao_ttnet_server_config.json`。

> 为什么是 `User Data`：`ahanet/` 目录 ↔ `aha_net.dll`；即**客户端自己落盘的 ttnet 服务端配置**，
> 明文 JSON，顶层 `{data, message:"success", summary}`。**纯静态即可取全，无需内存转储或 hook。**

## 1. `encrypt_config_v1` 全量值

```json
{"hs_in_t":120,"bk_k":"vcqSP1f9cE+jekl4+a6OghVX2yHJyYuTfHcDLOBXxLI=",
 "hs_en_t":600,"lead_ratio":0.9,"wildchar_hosts":["browser.doubao.com"],
 "bk_t":"MIGOEwdkZWZhdWx0AgUAoTaX0RMQZmFsbGJhY2tmNzY4ZDQ2YQQMPTc7sO+1wUhwrxZkAgIQAQQwLTfB7Z8EGwalfaa9mHPkcjsAvtM890fKuoqI085mSFM7S4Ac52KNiFJb11k6kdazBCBgh3YtBBFZCIqlZd+rmIs/ncfLd9OVIGPfJjBGxEjWzgIEaNKO0Q==",
 "hs_f_cnt":5,"retry_enabled":1,"wait_to":8,"fresh_ratio":0.7,"to":30,
 "url":"https://keyhub.zijieapi.com/handshake","ver":3,"l_lead_ratio":0.99,
 "l_exp":900,"s_exp":600,"enabled":1}
```

### 1.1 字段语义（与 `tt_encrypt_key_manager.cc` 内同名配置键对齐）

| 键 | 值 | 语义 |
|---|---|---|
| `enabled` / `ver` | 1 / 3 | 加密通道开关；配置版本 3 |
| `url` | keyhub 握手端点 | **握手唯一入口** |
| `bk_k` | base64 → **32 字节**（`bdca923f…e057c4b2`） | 基钥：256-bit 前置主密钥 |
| `bk_t` | base64 → **142 字节 DER 结构** | 基钥票据（见 §1.2） |
| `wildchar_hosts` | `["browser.doubao.com"]` | 加密生效的域名通配集合 |
| `hs_in_t` / `hs_en_t` / `hs_f_cnt` / `retry_enabled` / `wait_to` | 120 / 600 / 5 / 1 / 8 | 握手间隔、失败阈值与重试 |
| `to` / `s_exp` / `l_exp` / `l_lead_ratio` | 30 / 600 / 900 / 0.99 | 超时、短/长票据有效期、提前换票比例 |
| `pp_enabled` / `lead_ratio` / `fresh_ratio` | (常量) / 0.9 / 0.7 | 预握手开关与提前量/新鲜会话比例 |
| `key_shares` / `cipher_suites` / `key_share` / `cipher_suite` / `secp256r1` | 字符串常量 | **协商形态：ECDH over P-256** |
| `ticket_exp` / `ticket_long` / `ticket_long_exp` | 字符串常量 | 会话票据族 |
| `504273700a3d721f` / `6a15c5844fffc436` | 16 位 hex 常量 | 紧邻 encrypt 配置键的固定字面量（默认值/盐，用途待定） |

### 1.2 `bk_t` 结构（DER 解码实测）

```
3081 8E                              SEQUENCE, len 142
  13 07   "default"                  PrintableString: 环境名
  02 05   00 A1 3697D113             INTEGER(5)
  13 10   "fallbackf768d46a"         PrintableString: 回退标识（f768d46a = 8 字节指纹）
  04 0C   3D373BB0EFB5C14870AF1664   OCTET STRING(12)
  02 02   1001                       INTEGER(2)
  04 30   2D37C1ED…CE6648533B4B80   OCTET STRING(48)
  1C 07   62 8D88525BD7593A91D6B3   0x1C = 非标准 tag（DER 无此类）
  04 20   6087762D041159088AA565DF…  OCTET STRING(32)
  02 04   68D28ED1                 INTEGER(4)
```

要点：DER 风格但含非标准 tag（`0x1C`）与嵌套 OCTET STRING，**不是标准 X.509/PKCS**；
内嵌可读串 `default`、`fallbackf768d46a` 说明它是"环境 + 回退票据"的自定义序列化。
与 `bk_k`（32B 裸钥）配对即"**基钥 + 基钥票据**"。

## 2. ttnet 全配置面（`data` 30 项）

| 键 | 值（实测） |
|---|---|
| `ttnet_http_dns_addr` | **30 主机 → VIP 列表**（`opt.doubao.com` → `101.126.57.15,101.126.57.17,122.14.229.102`；`api5-normal-gl.doubao.com` → `221.194.168.229,221.194.168.230`）——HTTPDNS 直连表 |
| `ttnet_http_dns_prefer` / `_enabled` / `_google` / `_timeout` / `ttnet_local_dns_time_out` | 1 / 1 / 0 / 2 / 120 |
| `ttnet_h2_config` | `h2_session_timeout_enabled:1`、`h2_session_check_enabled:1`、`session_check_hosts:[api-normal/www/feishu/mcs/opt .doubao.com:443]` + ping keepalive 组（8 键） |
| `ttnet_quic_enabled` / `ttnet_quic_hint` / `ttnet_quic_internal_param` | 1 / `api5-normal-{xh,zb,lf,gl,hl,gl2}.doubao.com:443` / `{quic_version:"Q043", enable_ietf_quic_0rtt_reinvoke_fix:1, enable_retry_connect_other_dns_vip:1, broken_delay:1, congestion_control_type:2}` |
| `ttnet_preconnect_urls` | `https://api-normal.doubao.com`、`https://www.doubao.com` |
| `ttnet_dispatch_actions`（8）+ `_epoch` | `{act_priority, action:"dispatch", param:{contain_group, dispatch_strategy:3, host_group:[api-normal/api3-normal/api5-normal.doubao.com], service_name:"api_n…"}}`；epoch=`-1982072934` |
| `ttnet_retry_force_httpdns_wildchar_hosts`（11） | `*.doubao.com`、`*.feishu.cn`、`*.feishucdn.com`、`*.larkoffice.com`、`*.larkenterprise.com`、`*.bytedance.com`、`*.snssdk.com`、`*.zijieapi.com`、`*.byteimg.com`、`*.larksuitecdn.com`、`*.larksuite.com` |
| `ttnet_retry_force_httpdns_error_list` / `ttnet_request_retry_error_list` | `[-101,-102,-105,-108,-109,-118]` / `[-21,-109,-126,-101,-105,-7,-15,-102,-103,-106,-100]` |
| `ttnet_request_retry_max_attempts` / `_delay_interval_ms` | 4 / 200 |
| `tt_compress` | `{enabled:1, prefix_path:["/chat/completion"]}`——**只压聊天补全路径** |
| `send_tnc_host_arrays` | `tnc0-alisc1.zijieapi.com`、`tnc0-aliec2.zijieapi.com`（TNC 探测上报） |
| 其余开关 | `md5_check:0`、`report_native_requestlog:1`、`ttnet_enable_cronet_request_report:1`、`opaque_data_enabled:0`、`chromium_open:1`、`wpad_enabled:0`、`clear_pool_enabled:1`、`ttnet_url_dispatcher_enabled:1`、`ttnet_tt_http_dns:1` |

**两张域名表界定作用域**：`encrypt_config_v1.wildchar_hosts`（加密通道）与 `ttnet_retry_force_httpdns_wildchar_hosts`（强制 HTTPDNS 直连）。

## 3. 加密请求头族（`x-tt-e-*` / `x-tt-s-*`）

| 头 | 备注 |
|---|---|
| `x-tt-e-k` / `-b` / `-t` / `-d` / `-p` / `-h` / `-r` / `-i` | 加密通道请求头族（k=key、t=bk_t 标记、b=body、p=padding、h=header；r/i 待定位） |
| `x-tt-d-d` / `x-tt-r-d` | 同上块 |
| `x-tt-e-ignore-status-code-verify` | 跳过状态码校验（调试/降级） |
| `x-ttnet-retry-encrypt` | 加密通道重试标记 |
| `x-tt-s-sign` / `x-tt-s-cert-sign` / `x-tt-s-s` / `x-tt-s-r` / `x-tt-s-m` | 签名族（sign / 证书签名 / 其余待定位） |
| `x-tt-encrypt-debug` / `x-tt-encrypt-succ` | 调试与成功标记 |

> 桌面端**无** x-argus/x-ladon 签名族；`x-tt-e-*`/`x-tt-s-*` 是 ttnet 自带加密通道头，
> 与鉴权头（Cookie + DPoP + msToken）**分层**。

## 4. 握手入口定位结论

- 对 `app/*.dll` 全量 `grep -a keyhub` → **零命中**；两个 pak 与既有 pak 提取物 → **零命中**。
- 但 **`User Data/ahanet/server.json` 里有**（`data.encrypt_config_v1.url`）。
- 对 doubao 全量安装目录 `grep -rl encrypt_config_v1` 命中两处：`app/sscronet.dll`（**键名常量**）与 `User Data/ahanet/server.json`（**真实值**）。

准确结论：**握手入口由远端下发、落盘为明文配置** → 纯静态逆向**能**拿到入口与全部参数。
同目录另有 `tt_net_config.config`（128B）、`node_race_results`、`prefs/`、`tt_file_st/`、`version` 等 ttnet 落盘状态。

## 5. 归属声明（防串味）

早期版本曾以 `DoubaoWork/re/agent_out_native/memscan_net.bin` 作为配置值来源 —— 该转储含版本串 **`DoubaoWork/2.31.`**
与产品标识 `DoubaoWork`×162 / `doubaowork`×140，属**另一个产品**的内存，不能作 doubao 证据。
实测两者 `encrypt_config_v1` **逐字一致**（url / bk_k / bk_t / 全部阈值相同）
→ 证明 doubao 与 DoubaoWork 2.31 **共用同一套 ttnet 控制面配置**。旧转储仅作交叉佐证。

## 6. 剩余工作

| 项 | 下一步 |
|---|---|
| 握手响应格式 | 配置面已全量掌握；只剩响应报文字段级格式 → 对 `keyhub.zijieapi.com` 做**一次性 MITM**（已确认走标准 HTTPS，无需 hook 进程） |
| `bk_k`→会话密钥派生 | 位置锁定 `tt_encrypt_key_manager.cc` + `tt_encrypt_util.cc`；算法族 ECDH(P-256)+HKDF(boringssl)；**KDF 具体输入需单函数反编译**（rea 对本体的体量不可用，见 06 §2） |

## 7. keyhub 握手协议

加密通道由 keyhub 握手建立会话密钥。

| 项 | 值 |
|---|---|
| 端点 | `POST https://keyhub.zijieapi.com/handshake` |
| 请求 | `{version, random, app_id, did, key_shares:[{curve:"secp256r1", pubkey}], cipher_suites:[4097]}` |
| 请求签名 | `x-tt-s-sign` = ECDSA(SHA256) over 请求 JSON |
| 响应 | `{version, random, key_share:{curve, pubkey}, cipher_suite, ticket, expire_time}` |
| cipher suite | 4097 (0x1001) = ChaCha20 |
| ticket | ASN.1 DER `SEQUENCE{ OCTET STRING(12) nonce, OCTET STRING(48) 密文, OCTET STRING(32) key/MAC }` |
| 会话密钥（KDF） | ECDH(`secp256r1`) 共享密钥 → **HKDF-SHA256**：salt = `client_random(32B) ‖ server_random(32B)`（64B），**info = `"4e30514609050cd3"`（16B 硬编码常量，即本机二进制里那个，见 [15](DOUBAO-15-ttnet加密帧层再深挖与函数定位尝试.md)）** → 32B ChaCha20 key |
| 会话加密 | ChaCha20 流；nonce 12B（`x-tt-e-p`，实现用 4B 计数器 0 + 12B nonce） |
| 加密头 | `x-tt-e-b=1` / `x-tt-e-t=<ticket>` / `x-tt-e-p=<nonce>` / `x-ss-stub=<密文 MD5 大写>` |

本机静态定位与之一致：`0x284890`（首飞构造器，字段 `version/random/app_id/did/curve/pubkey/key_shares/cipher_suites`）、
`0x284eb0`（响应处理，字段 `cert/key_share/pubkey/curve/cipher_suite/ticket/ticket_exp`）、`x-tt-e-*` / `x-tt-s-sign` 头族、`bk_t` 142B DER。

**触发条件**：`wildchar_hosts` 里的 `browser.doubao.com` 是 **Chromium 账号同步后端**（`Doubao.dll` 内 `components\sync\base\sync_util.cc` 的 `sync_url = https://browser.doubao.com`）；只有触发内置浏览器的**账号同步**才会请求它，故常规使用下加密通道不 engage。

来源：`doubaoime-asr` 的 `wave_protocol.md`（多镜像）——
GitHub：https://github.com/starccy/doubaoime-asr/blob/main/wave_protocol.md ；
全文镜像（可直取）：https://context7.com/starccy/doubaoime-asr/llms.txt ；
npm 包：https://www.npmjs.com/package/doubaoime-asr

**触发链（本机反汇编）**：

```
0x262d90  TtNetworkDelegate::OnBeforeURLRequest (net\tt_net\traffic_control\tt_network_delegate.cc, 每请求前)
  → 0x281ad0 → 0x27fc20 / 0x2807d0 (编排)
    → 0x280d60 (发送: JSON + POST + x-tt-s-sign 签名)
      → 0x284890 (首飞构造) + 0x284eb0 (响应回调)
```

握手在**请求 host 命中 `encrypt_config_v1.wildchar_hosts`（`browser.doubao.com`）时**发起；不命中不握手。