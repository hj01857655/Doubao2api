# Reqable 抓包完整清单（去重）

> 生成自 `Reqable\box\data.mdb` 的 332 条会话记录，覆盖 132 个唯一端点。
> **脱敏纪律**：本文件只含方法/主机/路径/头名/参数名与计数；不含任何 header 值、cookie、token、query 值。

## 1. 主机分布

| host | 会话数 |
|---|---|
| `www.doubao.com` | 165 |
| `mcs.doubao.com` | 66 |
| `accounts.doubao.com` | 29 |
| `lf-flow-web-cdn.doubao.com` | 27 |
| `passport.feishu.cn` | 6 |
| `internal-api-lark-api.feishu.cn` | 5 |
| `lf3-config.bytetcc.com` | 4 |
| `wss100-normal.doubao.com` | 4 |
| `gecko.zijieapi.com` | 3 |
| `www.feishu.cn` | 3 |
| `id1.cloud.huawei.com` | 2 |
| `api-normal.doubao.com` | 2 |
| `accounts.feishu.cn` | 2 |
| `log.zijieapi.com` | 2 |
| `lf3-short.ibytedapm.com` | 2 |
| `codearts.huaweicloud.com` | 1 |
| `oauth-login1.cloud.huawei.com` | 1 |
| `tnc0-alisc1.zijieapi.com` | 1 |
| `log-klink.zijieapi.com` | 1 |
| `logifier.doubao.com` | 1 |
| `update.googleapis.com` | 1 |
| `lf26-cdn-tos.bytegecko.com` | 1 |
| `doh.zjurl.cn` | 1 |
| `i.snssdk.com` | 1 |
| `p3-flow-imagex-sign.byteimg.com` | 1 |

## 2. 端点全清单（132 个，按调用次数降序）

| # | 次数 | 端点 | query 参数名（去重，无值） | 带 `a_bogus` |
|---|---|---|---|---|
| 1 | 50 | `POST mcs.doubao.com/list` | — | - |
| 2 | 17 | `POST www.doubao.com/im/chain/recent_conv` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 3 | 9 | `POST www.doubao.com/alice/user/config/pull` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 4 | 8 | `POST www.doubao.com/ttwid/check/` | — | - |
| 5 | 6 | `GET accounts.doubao.com/passport/account/info/v2/` | `a_bogus`, `account_sdk_source`, `aid`, `channel`, `chromium_version`, `device_id`, `device_platform`, `doubao_from`, `iid`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `sdk_version`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version`, `version_code` | 是 |
| 6 | 6 | `POST mcs.doubao.com/service/2/abtest_config/` | — | - |
| 7 | 6 | `POST www.doubao.com/im/project/list` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 8 | 6 | `GET www.doubao.com/samantha/notice/info` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 9 | 6 | `POST www.doubao.com/alice/commerce/sale/subscription/entry/config/` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 10 | 5 | `POST www.doubao.com/alice/profile/self` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `iid`, `is_new_user`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 11 | 5 | `POST mcs.doubao.com/tobid` | — | - |
| 12 | 5 | `POST www.doubao.com/alice/slot/action_bar_v3/get_item_conf` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 13 | 5 | `POST mcs.doubao.com/v1/list` | `aid`, `channel`, `chromium_version`, `pc_version`, `real_aid`, `runtime`, `version` | - |
| 14 | 4 | `GET lf3-config.bytetcc.com/obj/tcc-config-web/tcc-v2-data-flow.web.doubao-default` | — | - |
| 15 | 4 | `POST www.doubao.com/samantha/user/ab/get` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 16 | 4 | `POST accounts.doubao.com/passport/doubao/security_token_beat/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_id`, `device_platform`, `did`, `iid`, `is_from_ttaccountsdk`, `language`, `msToken`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `sign`, `terminal_type`, `ts`, `version_code` | 是 |
| 17 | 4 | `POST accounts.doubao.com/passport/web/auth/get_oauth_token/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_id`, `device_platform`, `did`, `iid`, `is_from_ttaccountsdk`, `language`, `msToken`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `sign`, `terminal_type`, `ts`, `version_code` | 是 |
| 18 | 4 | `POST www.doubao.com/alice/commerce/marketing/card/balance/` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 19 | 4 | `POST www.doubao.com/alice/commerce/sale/subscription/quota/summary/` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 20 | 4 | `POST www.doubao.com/im/conversation/batch_get` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 21 | 4 | `GET wss100-normal.doubao.com/ws/v2` | `access_key`, `aid`, `channel`, `chromium_version`, `client_platform`, `conn_tag`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `fpid`, `language`, `pc_version`, `pkg_type`, `qos_sdk_version`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_did_conn`, `web_id`, `web_platform`, `web_tab_id`, `xaack`, `xsack`, `xsqos` | - |
| 22 | 4 | `POST www.doubao.com/im/message/chain_by_block` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 23 | 3 | `POST www.doubao.com/alice/theme/config` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `use-olympus-account`, `version_code`, `web_platform`, `web_tab_id` | - |
| 24 | 3 | `POST www.doubao.com/alice/user/launch/core` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `use-olympus-account`, `version_code`, `web_platform`, `web_tab_id` | - |
| 25 | 3 | `POST www.doubao.com/alice/slot/action_bar_v3/brief_list` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 26 | 3 | `POST www.doubao.com/alice/bot/action_bar_v3/active_switch_banner` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 27 | 3 | `GET accounts.doubao.com/passport/safe/csrf_token/` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 28 | 3 | `POST passport.feishu.cn/accounts/csrf` | `_t` | - |
| 29 | 3 | `GET www.doubao.com/passport/token/beat/web/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `is_from_ttaccountsdk`, `language`, `msToken`, `os_type`, `p_bd`, `p_js_t`, `p_js_v`, `p_no`, `p_ts`, `p_ver`, `p_ver_real`, `p_zt`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `request_host`, `scene`, `sign`, `terminal_type`, `ts`, `version` | 是 |
| 30 | 3 | `GET passport.feishu.cn/accounts/web/user` | `_t`, `app_id`, `support_anonymous` | - |
| 31 | 3 | `POST www.doubao.com/alice/user/launch` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 32 | 3 | `POST gecko.zijieapi.com/gecko/server/v3/package` | — | - |
| 33 | 3 | `POST www.doubao.com/im/conversation/abstract` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 34 | 3 | `GET www.feishu.cn/space/api/infra/domains/all` | — | - |
| 35 | 2 | `CONNECT id1.cloud.huawei.com/` | — | - |
| 36 | 2 | `GET www.doubao.com/samantha/fission/entrance` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `enter_from`, `fp`, `iid`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 37 | 2 | `POST www.doubao.com/samantha/bookmark/update` | `aid`, `channel`, `chromium_version`, `pc_version`, `real_aid`, `runtime`, `version` | - |
| 38 | 2 | `POST www.doubao.com/alice/office/file_sync/get_files` | `aid`, `channel`, `chromium_version`, `pc_version`, `real_aid`, `runtime`, `version` | - |
| 39 | 2 | `GET www.doubao.com/samantha/bookmark` | `aid`, `channel`, `chromium_version`, `pc_version`, `real_aid`, `runtime`, `version` | - |
| 40 | 2 | `GET www.doubao.com/check_and_get_text/00d51bf0012811ee97fd5fdb0b0365a9/normal/web` | `lang`, `version` | - |
| 41 | 2 | `POST www.doubao.com/samantha/skill/list` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `use-olympus-account`, `version_code`, `web_platform`, `web_tab_id` | - |
| 42 | 2 | `POST www.doubao.com/samantha/user/setting/get` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `use-olympus-account`, `version_code`, `web_platform`, `web_tab_id` | - |
| 43 | 2 | `POST www.doubao.com/samantha/user/preference/get` | `aid`, `channel`, `chromium_version`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `iid`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 44 | 2 | `POST www.doubao.com/app_framework/sidebar` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 45 | 2 | `POST www.doubao.com/alice/search/launch` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 46 | 2 | `POST accounts.feishu.cn/accounts/status/heartbeat` | — | - |
| 47 | 2 | `POST accounts.doubao.com/passport/user/biz_auth/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_id`, `device_platform`, `did`, `iid`, `is_from_ttaccountsdk`, `language`, `msToken`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `sign`, `terminal_type`, `ts`, `version_code` | 是 |
| 48 | 2 | `GET www.doubao.com/alice/dispatch/all_devices` | `a_bogus`, `aid`, `all_pc_2`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 49 | 2 | `GET internal-api-lark-api.feishu.cn/settings/v3/` | `action`, `app`, `app_channel`, `app_id`, `platform`, `version` | - |
| 50 | 2 | `POST www.doubao.com/alice/office/skills/list_user_and_featured` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 51 | 2 | `POST www.doubao.com/alice/call/downgrade_config_pc` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 52 | 2 | `POST www.doubao.com/alice/im/launch` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 53 | 2 | `POST www.doubao.com/alice/basic/launch` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 54 | 2 | `POST www.doubao.com/samantha/skill/recommend` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 55 | 2 | `POST www.doubao.com/samantha/plugin/prompt_skill/list_by_user` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 56 | 2 | `POST www.doubao.com/im/group/list` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 57 | 2 | `POST www.doubao.com/alice/user/get_prefered_lark_account_info` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 58 | 2 | `POST www.doubao.com/im/message/send_rate_limit` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 59 | 2 | `POST www.doubao.com/alice/commerce/intake/entitlement/query` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 60 | 2 | `OPTIONS accounts.doubao.com/passport/account/info/v2/` | `a_bogus`, `account_sdk_source`, `aid`, `device_platform`, `doubao_from`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `sdk_version`, `sys_region`, `use-olympus-account`, `version_code` | 是 |
| 61 | 2 | `POST www.doubao.com/im/chain/single` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 62 | 2 | `POST www.doubao.com/im/conversation/info` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 63 | 1 | `CONNECT codearts.huaweicloud.com/` | — | - |
| 64 | 1 | `CONNECT oauth-login1.cloud.huawei.com/` | — | - |
| 65 | 1 | `GET tnc0-alisc1.zijieapi.com/get_domains/v5/` | `aid`, `app_name`, `cronet_version`, `delay`, `device_id`, `device_platform`, `tnc_src`, `ttnet_version`, `version_code` | - |
| 66 | 1 | `POST www.doubao.com/alice/user/get_web_anon_id` | `aid`, `channel`, `chromium_version`, `device_id`, `device_platform`, `iid`, `language`, `pc_version`, `real_aid`, `runtime`, `samantha_web`, `tea_uuid`, `use-olympus-account`, `version`, `version_code` | - |
| 67 | 1 | `GET www.doubao.com/service/settings/v3/` | `aid`, `brand`, `channel`, `chromium_version`, `cpu_count`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_os_version`, `doubao_pc_version`, `enterprise_user_type`, `first_run_time`, `iid`, `is_bd_env`, `kernal_version`, `language`, `machine_memory`, `os`, `os_version`, `pc_runtime`, `pc_version`, `pc_version_num`, `real_aid`, `runtime`, `scale_factor`, `tea_uuid`, `update_channel`, `user_id`, `user_unique_id`, `utm_source`, `version`, `version_code` | - |
| 68 | 1 | `GET www.doubao.com/samantha/desktop/get_conflict_app_blocklist` | — | - |
| 69 | 1 | `POST api-normal.doubao.com/service/2/desktop/device_register/` | `aid`, `channel`, `device_platform`, `os`, `os_version`, `pc_serial`, `pc_uuid`, `version_code` | - |
| 70 | 1 | `POST internal-api-lark-api.feishu.cn/accounts/csrf` | `_t` | - |
| 71 | 1 | `POST www.doubao.com/alice/dispatch/report_device` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 72 | 1 | `GET internal-api-lark-api.feishu.cn/accounts/web/user` | `_t`, `app_id`, `support_anonymous` | - |
| 73 | 1 | `GET internal-api-lark-api.feishu.cn/settings/unbundle/` | `action`, `app`, `app_channel`, `app_id`, `platform`, `tenant_id`, `user_id`, `version` | - |
| 74 | 1 | `POST log-klink.zijieapi.com/service/2/device_register/` | `aid`, `cdid`, `channel`, `device_id`, `device_platform`, `iid`, `os_version`, `tt_data` | - |
| 75 | 1 | `POST api-normal.doubao.com/service/2/app_alert_check/` | `aid`, `app_name`, `channel`, `device_id`, `device_platform`, `iid`, `os`, `pc_serial`, `pc_uuid`, `version_code` | - |
| 76 | 1 | `POST log.zijieapi.com/service/2/log_settings/` | `aid`, `cdid`, `channel`, `device_id`, `device_platform`, `iid`, `os_version`, `tt_data` | - |
| 77 | 1 | `POST log.zijieapi.com/service/2/app_log/` | `aid`, `cdid`, `channel`, `device_id`, `device_platform`, `iid`, `os_version`, `tt_data` | - |
| 78 | 1 | `GET www.doubao.com/alice/dispatch/setting/user` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 79 | 1 | `POST www.doubao.com/alice/user/report_user_behavior` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 80 | 1 | `GET logifier.doubao.com/logifier/retrieval/tasks/poll` | `query`, `sign`, `ts` | - |
| 81 | 1 | `POST www.doubao.com/biz/activity/get_push_banner` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 82 | 1 | `POST www.doubao.com/biz/activity/get_pop_window` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 83 | 1 | `GET www.doubao.com/drive-iframe/drive/home/` | `drive_iframe_defer_render`, `drive_iframe_desktop_host`, `drive_iframe_language`, `drive_sidebar_placeholder_width`, `theme` | - |
| 84 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/css/l.5758f112d3.css` | — | - |
| 85 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/css/m5.827dfc306d.css` | — | - |
| 86 | 1 | `GET lf3-short.ibytedapm.com/slardar/fe/sdk-web/browser.1.16.6.cn.js` | `bid`, `globalName` | - |
| 87 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/lib-polyfill.7c0bf2cd5a.js` | — | - |
| 88 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/m5.bb02644af6.js` | — | - |
| 89 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/tif.909c5e7144.js` | — | - |
| 90 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/qnd.a6bf4705b4.js` | — | - |
| 91 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/a96.18d4621486.js` | — | - |
| 92 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/4p.a1dd95a59b.js` | — | - |
| 93 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/drive-iframe.0415e654b9.js` | — | - |
| 94 | 1 | `POST www.doubao.com/alice/office/asset/import/config/get` | `aid`, `channel`, `chromium_version`, `pc_version`, `real_aid`, `runtime`, `version` | - |
| 95 | 1 | `POST update.googleapis.com/service/update2/json` | `cup2hreq`, `cup2key` | - |
| 96 | 1 | `POST www.doubao.com/alice/slot/action_bar_v3/thinkmode_memory_report` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 97 | 1 | `GET lf3-short.ibytedapm.com/slardar/fe/sdk-web/plugins/common-monitors.1.16.6.js` | — | - |
| 98 | 1 | `POST www.doubao.com/im/conversation/modify` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 99 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/s1-chat-runtime-after-visible.8f37f697a5.js` | — | - |
| 100 | 1 | `POST www.doubao.com/chat/completion` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `tz_name`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 101 | 1 | `GET www.doubao.com/partner/ccm-office/preheat` | `clientId`, `instanceId`, `preheat`, `preheatBiz`, `useMessageV2` | - |
| 102 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/bm2.0e13705659.js` | — | - |
| 103 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/e4.41180e4bc9.js` | — | - |
| 104 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/7hf.8660d9e249.js` | — | - |
| 105 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/qx8.95189e9056.js` | — | - |
| 106 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/ej.53ec5a9f93.js` | — | - |
| 107 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/s2-chat-runtime.747cae5610.js` | — | - |
| 108 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/t3c.55d64d7136.js` | — | - |
| 109 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/s2-desktop-shell-api.84dcd5518f.js` | — | - |
| 110 | 1 | `GET accounts.doubao.com/passport/account/list/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_id`, `device_platform`, `did`, `iid`, `is_from_ttaccountsdk`, `language`, `msToken`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `scene`, `sign`, `terminal_type`, `ts`, `version_code` | 是 |
| 111 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/s2-account-shell-api.b9602a6518.js` | — | - |
| 112 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/vh.b96f7eb4e6.js` | — | - |
| 113 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/yn.57d1cce233.js` | — | - |
| 114 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/5yf.4e2433a1e5.js` | — | - |
| 115 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/w9.23547b5fb3.js` | — | - |
| 116 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/q9e.ce198a7f6a.js` | — | - |
| 117 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/pabb.337cb147ed.js` | — | - |
| 118 | 1 | `POST www.doubao.com/im/message/mark_conv_read` | `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | - |
| 119 | 1 | `OPTIONS accounts.doubao.com/ttwid/check/` | — | - |
| 120 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao/web/static/js/async/x2.8da7c52037.js` | — | - |
| 121 | 1 | `POST www.doubao.com/alice/profile/multi` | `a_bogus`, `aid`, `channel`, `chromium_version`, `client_platform`, `device_id`, `device_platform`, `doubao_device_platform`, `doubao_pc_version`, `fp`, `language`, `msToken`, `pc_version`, `pkg_type`, `real_aid`, `region`, `runtime`, `runtime_version`, `samantha_web`, `sys_region`, `tea_uuid`, `use-olympus-account`, `version_code`, `web_id`, `web_platform`, `web_tab_id` | 是 |
| 122 | 1 | `POST accounts.doubao.com/ttwid/check/` | — | - |
| 123 | 1 | `OPTIONS accounts.doubao.com/passport/token/beat/web/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `is_from_ttaccountsdk`, `language`, `os_type`, `p_bd`, `p_js_t`, `p_js_v`, `p_no`, `p_ts`, `p_ver`, `p_ver_real`, `p_zt`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `request_host`, `scene`, `sign`, `terminal_type`, `ts`, `version` | 是 |
| 124 | 1 | `GET accounts.doubao.com/passport/token/beat/web/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `is_from_ttaccountsdk`, `language`, `os_type`, `p_bd`, `p_js_t`, `p_js_v`, `p_no`, `p_ts`, `p_ver`, `p_ver_real`, `p_zt`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `request_host`, `scene`, `sign`, `terminal_type`, `ts`, `version` | 是 |
| 125 | 1 | `OPTIONS accounts.doubao.com/passport/doubao/security_token_beat/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_platform`, `is_from_ttaccountsdk`, `language`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `sign`, `terminal_type`, `ts` | 是 |
| 126 | 1 | `OPTIONS accounts.doubao.com/passport/user/biz_auth/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_platform`, `is_from_ttaccountsdk`, `language`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `sign`, `terminal_type`, `ts` | 是 |
| 127 | 1 | `OPTIONS accounts.doubao.com/passport/web/auth/get_oauth_token/` | `a_bogus`, `account_app_language`, `account_sdk_source`, `account_sdk_source_info`, `aid`, `biz_trace_id`, `device_platform`, `is_from_ttaccountsdk`, `language`, `multi_login`, `os_type`, `passport_jssdk_type`, `passport_jssdk_version`, `qs`, `sign`, `terminal_type`, `ts` | 是 |
| 128 | 1 | `GET lf26-cdn-tos.bytegecko.com/obj/ies.fe.gecko/11597/179074806/5f8a7c6f866b6be4d429d406fe84ca46_7697229308128968742.zip` | — | - |
| 129 | 1 | `GET doh.zjurl.cn/q` | `host`, `ttl` | - |
| 130 | 1 | `POST i.snssdk.com/cloudpush/update_sender/` | `aid`, `device_id`, `os`, `push_sdk`, `tz_name`, `tz_offset`, `version_code` | - |
| 131 | 1 | `GET lf-flow-web-cdn.doubao.com/obj/flow-doubao/doubao_pc/2.32.6/Doubao_win_X64_2.32.6-signed.zip` | — | - |
| 132 | 1 | `GET p3-flow-imagex-sign.byteimg.com/tos-cn-i-a9rns2rl98/3215fc194bc2207c6d213e5610bb7b14.png~tplv-a9rns2rl98-image-qvalue.png` | `lk3s`, `rcl`, `rrcfp`, `x-expires`, `x-signature` | - |

## 3. 请求头名全集（56 个，值不落盘）

| 头名 | 出现次数 |
|---|---|
| `cookie` | 7116 |
| `host` | 332 |
| `user-agent` | 332 |
| `accept-encoding` | 327 |
| `sec-fetch-site` | 318 |
| `sec-fetch-mode` | 318 |
| `sec-fetch-dest` | 318 |
| `accept-language` | 309 |
| `priority` | 292 |
| `accept` | 288 |
| `referer` | 288 |
| `sec-ch-ua-platform` | 281 |
| `sec-ch-ua` | 281 |
| `sec-ch-ua-mobile` | 281 |
| `content-length` | 236 |
| `content-type` | 229 |
| `agw-js-conv` | 147 |
| `sec-fetch-storage-access` | 59 |
| `origin` | 52 |
| `cache-control` | 15 |
| `x-tt-passport-aid-sign` | 15 |
| `x-tt-passport-csrf-token` | 15 |
| `x-tt-passport-trace-id` | 15 |
| `x-tt-multi-sids` | 11 |
| `x-app-id` | 9 |
| `x-device-info` | 8 |
| `x-request-id` | 8 |
| `x-terminal-type` | 8 |
| `x-api-version` | 8 |
| `access-control-request-method` | 7 |
| `access-control-request-headers` | 7 |
| `if-none-match` | 6 |
| `if-modified-since` | 6 |
| `connection` | 5 |
| `pragma` | 5 |
| `proxy-connection` | 4 |
| `x-locale` | 4 |
| `upgrade` | 4 |
| `sec-websocket-version` | 4 |
| `sec-websocket-key` | 4 |
| `sec-websocket-extensions` | 4 |
| `sec-websocket-protocol` | 4 |
| `authorization` | 3 |
| `dpop` | 3 |
| `x-ls-caller-type` | 3 |
| `upgrade-insecure-requests` | 2 |
| `x-flow-chromium-version` | 2 |
| `x-flow-device-id` | 2 |
| `local-etag` | 1 |
| `x-tt-tnc-summary` | 1 |
| `x-goog-update-appid` | 1 |
| `x-goog-update-interactivity` | 1 |
| `x-goog-update-updater` | 1 |
| `x-flow-trace` | 1 |
| `last-event-id` | 1 |
| `x-rust-request-timeout` | 1 |

## 4. 响应头名全集（108 个）

| 头名 | 出现次数 |
|---|---|
| `server-timing` | 506 |
| `date` | 323 |
| `server` | 323 |
| `x-tt-logid` | 322 |
| `x-tt-trace-host` | 322 |
| `x-tt-trace-id` | 322 |
| `x-tt-trace-tag` | 322 |
| `via` | 319 |
| `content-type` | 306 |
| `set-cookie` | 292 |
| `x-tt-timestamp` | 261 |
| `x-tt-supplier-id` | 200 |
| `vary` | 196 |
| `content-encoding` | 178 |
| `tt-idc-switch` | 173 |
| `content-length` | 171 |
| `x-hcs-proxy-type` | 166 |
| `x-ccdn-cachettl` | 166 |
| `x-ccdn-req-id-46b1` | 166 |
| `x-request-ip` | 130 |
| `x-tt-agw-login` | 121 |
| `tt_stable` | 116 |
| `strict-transport-security` | 105 |
| `cache-control` | 100 |
| `x-dsa-trace-id` | 98 |
| `x-dsa-origin-status` | 98 |
| `access-control-allow-methods` | 85 |
| `access-control-allow-credentials` | 78 |
| `access-control-max-age` | 73 |
| `upstream-caught` | 61 |
| `accept-ranges` | 60 |
| `cross-origin-resource-policy` | 60 |
| `access-control-allow-origin` | 57 |
| `age` | 56 |
| `pragma` | 56 |
| `expires` | 56 |
| `timing-allow-origin` | 56 |
| `eagleid` | 54 |
| `x-ms-token` | 38 |
| `etag` | 37 |
| `x-tos-hash-crc64ecma` | 34 |
| `x-tos-request-id` | 34 |
| `x-tos-response-time` | 34 |
| `x-tos-storage-class` | 34 |
| `x-response-cache` | 34 |
| `last-modified` | 34 |
| `x-bdcdn-cache-status` | 32 |
| `x-kfc-cachekey` | 32 |
| `x-response-cinfo` | 32 |
| `ali-swift-global-savetime` | 32 |
| `x-cache` | 32 |
| `x-swift-savetime` | 32 |
| `x-swift-cachetime` | 32 |
| `x-tt-flow-login` | 27 |
| `bd-ticket-guard-key-sign-result` | 24 |
| `bd-ticket-guard-sign-res-static-ts-sign` | 24 |
| `bd-ticket-guard-sign-res-static-sign` | 24 |
| `x-tt-e-b` | 20 |
| `access-control-allow-headers` | 18 |
| `x-envoy-response-flags` | 16 |
| `x-request-id` | 16 |
| `x-timestamp` | 16 |
| `access-control-expose-headers` | 16 |
| `bd-tt-error-code` | 14 |
| `x-lgw-dst-svc` | 13 |
| `request-id` | 13 |
| `to-env` | 8 |
| `content-md5` | 8 |
| `proxy-status` | 8 |
| `access-control-request-methods` | 6 |
| `connection` | 5 |
| `x-tt-cip` | 5 |
| `bd-ticket-guard-identity-token` | 4 |
| `bd-ticket-guard-sec-ts` | 4 |
| `bd-ticket-guard-ts` | 4 |
| `x-tt-passport-tlb-tag-result` | 4 |
| `upgrade` | 4 |
| `sec-websocket-accept` | 4 |
| `handshake-msg` | 4 |
| `handshake-options` | 4 |
| `handshake-status` | 4 |
| `sec-websocket-protocol` | 4 |
| `x-frontier-logid` | 4 |
| `x-alicdn-da-ups-status` | 3 |
| `content-security-policy` | 3 |
| `x-lsc-source-ip` | 3 |
| `x-nws-log-uuid` | 2 |
| `x-cache-lookup` | 2 |
| `x-ggw-config-version` | 2 |
| `content-security-policy-report-only` | 2 |
| `reporting-endpoints` | 2 |
| `x-server` | 2 |
| `x-ss-etag` | 1 |
| `x-tt-tnc-control` | 1 |
| `x-agw-info` | 1 |
| `x-pod-name` | 1 |
| `x-server-ts` | 1 |
| `x-cup-server-proof` | 1 |
| `x-daynum` | 1 |
| `x-daystart` | 1 |
| `x-content-type-options` | 1 |
| `x-frame-options` | 1 |
| `x-xss-protection` | 1 |
| `alt-svc` | 1 |
| `x-tos-meta-gkfs-block-indexing-url` | 1 |
| `x-tos-meta-gkfs-block-meta` | 1 |
| `nginx-hit` | 1 |
| `x-ccdn-expires` | 1 |

## 5. query 参数名全集（125 个）

`aid`(195), `device_platform`(177), `language`(171), `device_id`(167), `channel`(164), `pc_version`(164), `real_aid`(161), `version_code`(160), `chromium_version`(159), `runtime`(159), `use-olympus-account`(148), `pkg_type`(146), `region`(146), `sys_region`(146), `samantha_web`(145), `doubao_device_platform`(145), `doubao_pc_version`(145), `web_platform`(144), `web_tab_id`(144), `client_platform`(141), `fp`(141), `runtime_version`(141), `tea_uuid`(126), `web_id`(120), `a_bogus`(95), `msToken`(82), `version`(28), `account_sdk_source`(26), `sign`(20), `ts`(20), `account_app_language`(19), `account_sdk_source_info`(19), `biz_trace_id`(19), `is_from_ttaccountsdk`(19), `os_type`(19), `passport_jssdk_type`(19), `passport_jssdk_version`(19), `qs`(19), `terminal_type`(19), `iid`(18), `multi_login`(14), `_t`(8), `did`(8), `sdk_version`(7), `app_id`(7), `scene`(6), `os_version`(5), `p_bd`(5), `p_js_t`(5), `p_js_v`(5), `p_no`(5), `p_ts`(5), `p_ver`(5), `p_ver_real`(5), `p_zt`(5), `request_host`(5), `os`(4), `support_anonymous`(4), `access_key`(4), `conn_tag`(4), `fpid`(4), `qos_sdk_version`(4), `web_did_conn`(4), `xaack`(4), `xsack`(4), `xsqos`(4), `action`(3), `app`(3), `app_channel`(3), `platform`(3), `cdid`(3), `tt_data`(3), `app_name`(2), `user_id`(2), `enter_from`(2), `lang`(2), `pc_serial`(2), `pc_uuid`(2), `all_pc_2`(2), `doubao_from`(2), `tz_name`(2), `cronet_version`(1), `delay`(1), `tnc_src`(1), `ttnet_version`(1), `brand`(1), `cpu_count`(1), `doubao_os_version`(1), `enterprise_user_type`(1), `first_run_time`(1), `is_bd_env`(1), `kernal_version`(1), `machine_memory`(1), `pc_runtime`(1), `pc_version_num`(1), `scale_factor`(1), `update_channel`(1), `user_unique_id`(1), `utm_source`(1), `is_new_user`(1), `tenant_id`(1), `query`(1), `drive_iframe_defer_render`(1), `drive_iframe_desktop_host`(1), `drive_iframe_language`(1), `drive_sidebar_placeholder_width`(1), `theme`(1), `bid`(1), `globalName`(1), `cup2hreq`(1), `cup2key`(1), `clientId`(1), `instanceId`(1), `preheat`(1), `preheatBiz`(1), `useMessageV2`(1), `host`(1), `ttl`(1), `push_sdk`(1), `tz_offset`(1), `lk3s`(1), `rcl`(1), `rrcfp`(1), `x-expires`(1), `x-signature`(1)

## 6. 关键判读

- `a_bogus` 只出现在 41 个端点上；**90 个端点完全不发送**它（含调用量第一的 `POST /im/chain/recent_conv`），且服务端照常返回 ⇒ 客户端按端点白名单添加，非服务端强制。
- `x-tt-*` 请求头仅出现在 passport 域（`x-tt-passport-aid-sign`/`x-tt-passport-csrf-token`/`x-tt-passport-trace-id`/`x-tt-multi-sids`）；**业务域 `www.doubao.com` 的请求头里没有任何 `x-tt-*`**。
- 响应侧 `x-ms-token` 出现 38 次 ⇒ 服务端轮转签发 msToken。
- 业务域出现的非标准请求头只有 `agw-js-conv`(147)、`last-event-id`、`x-flow-trace`，与 docs/03 记录一致。

## 7. 状态码分布

| status | 会话数 |
|---|---|
| `(none)` | 332 |
