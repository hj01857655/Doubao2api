// Package doubao 对应 Python app/doubao.py — 豆包 /chat/completion 上游客户端。
//
// 契约来源：docs/03（请求面）、docs/04 + docs/05（响应面与探测修正）。
// 设计要点（都是实测结论，勿凭感觉改）：
//
//  1. 不实现任何签名。a_bogus/msToken 在本端点不被校验（docs/05 §1.1）。
//  2. 设备 query 必须成套自洽，砍参数会触发风控滑块 710022004（docs/05 §1.3）。
//  3. 鉴权只需 10 个 cookie 名（docs/05 §1.2）。
//  4. 正文片段可能来自 CHUNK_DELTA 或 STREAM_CHUNK(patch_object:1)，二者不重叠，
//     拼接即权威全文。end_type:1 的 msg_finish_attr.brief 只是会话列表预览串
//     （实测会吃换行、长回答截断）→ 绝不用于覆盖正文。
//  5. end_type:1 = 正文已完整（此时即可发 finish_reason）；end_type:3 = 流真正结束。
//     实测两者相差 2~3 秒，故解析器在 end_type:1 额外发 body_end 事件。
//  6. 错误有三支：HTTP 非 200、event: gateway-error、event: STREAM_ERROR。
package doubao

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	HOST            = "www.doubao.com"
	COMPLETION_PATH = "/chat/completion"
)

// ── 设备面常量 ────────────────────────────────────────────────────────────────
// 与抓包同一套自洽值。fp = verify_<device_id>、tea_uuid = device_id 是强约束，
// 三者不一致会被风控判成异常设备（docs/05 §1.3）。可用环境变量覆盖以便多账号轮换。

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var (
	DEVICE_ID        = envOr("DOUBAO_DEVICE_ID", "854749611058841")
	WEB_ID           = envOr("DOUBAO_WEB_ID", "7696568307852379690")
	PC_VERSION       = envOr("DOUBAO_PC_VERSION", "2.32.5")
	CHROMIUM_VERSION = envOr("DOUBAO_CHROMIUM_VERSION", "147.0.7727.149")
	USER_AGENT       = fmt.Sprintf(
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "+
			"(KHTML, like Gecko) Chrome/%s Safari/537.36 SamanthaDoubao/%s",
		chromiumMajor(), PC_VERSION)
)

func chromiumMajor() string {
	if i := strings.Index(CHROMIUM_VERSION, "."); i >= 0 {
		return CHROMIUM_VERSION[:i]
	}
	return CHROMIUM_VERSION
}

// webTabID 每进程一个，随会话变化；这里固定即可（对齐 Python 模块加载时生成一次）。
var webTabID = sync.OnceValue(uuidV4)

// BASE_QUERY 设备面 query，保序（Python dict 序列化顺序即抓包顺序）。
func baseQuery() [][2]string {
	return [][2]string{
		{"aid", "582478"},
		{"real_aid", "582478"},
		{"channel", "win"},
		{"chromium_version", CHROMIUM_VERSION},
		{"client_platform", "pc_client"},
		{"device_id", DEVICE_ID},
		{"device_platform", "web"},
		{"doubao_device_platform", "desktop"},
		{"doubao_pc_version", PC_VERSION},
		{"fp", "verify_" + DEVICE_ID},
		{"language", "zh"},
		{"pc_version", PC_VERSION},
		{"pkg_type", "release_version"},
		{"region", "CN"},
		{"runtime", "web"},
		{"runtime_version", "3.41.2"},
		{"samantha_web", "1"},
		{"sys_region", "CN"},
		{"tea_uuid", DEVICE_ID},
		{"tz_name", "Asia/Shanghai"},
		{"use-olympus-account", "1"},
		{"version_code", "20800"},
		{"web_id", WEB_ID},
		{"web_platform", "desktop"},
		{"web_tab_id", webTabID()},
	}
}

// 鉴权最小集。docs/05 §1.2 的 10 名是「只保留这些也 PASS」的上限；2026-10-25
// 扫码实测（一次性探针，已删）：缺 d_ticket / s_v_web_id 的 8 名直接打真实
// chat 端点 PASS。这两个名在登录回跳链上根本不下发——s_v_web_id 是客户端
// 自造的 verify_ 形态设备指纹，d_ticket 是 SSO 支撑票，均非必需，降级为可选。
var REQUIRED_COOKIES = []string{
	"sessionid", "sessionid_ss", "sid_tt", "sid_guard", "uid_tt", "uid_tt_ss",
	"ttwid", "passport_csrf_token",
}

// OPTIONAL_COOKIES 手动配置场景里可能带来、但实测非必需的 cookie，有则带上。
var OPTIONAL_COOKIES = []string{"d_ticket", "s_v_web_id"}

// ── 模型映射 ──────────────────────────────────────────────────────────────────
// 来源：/alice/slot/action_bar_v3/brief_list 的 active_switch_conf。
// reasoning_effort 实测默认 "3"（docs/03 §3.1 conversation_init_ext）。

// Mode 一次对话的档位面参数。
type Mode struct {
	ModelItemKey     string // conversation_init_ext.model_item_key
	ModeID           string // conversation_init_ext.mode_id
	ConversationMode int    // option.conversation_mode
	AgentMode        int    // option.agent_mode
	ReasoningEffort  int
	Display          string
}

var MODES = map[string]Mode{
	// 对话（chat，mode_id=1）
	"doubao-fast":   {"0", "1", 1, 2, 3, "豆包 快速"},
	"doubao-expert": {"3", "1", 1, 2, 3, "豆包 专家"},
	// 工作（office，mode_id=3）
	"doubao-auto":     {"9", "3", 3, 2, 3, "自动"},
	"doubao-turbo":    {"4", "3", 3, 2, 3, "豆包 2.1 Turbo"},
	"doubao-pro":      {"5", "3", 3, 2, 3, "豆包 2.1 Pro"},
	"doubao-2.1-lite": {"seed-lite-7b", "3", 3, 2, 3, "豆包 2.1 Lite"},
}

const DEFAULT_MODEL = "doubao-fast"

// MODEL_ALIASES 兼容 OpenAI 习惯名。
var MODEL_ALIASES = map[string]string{
	"doubao":      DEFAULT_MODEL,
	"doubao-pro":  "doubao-pro",
	"doubao-seed": "doubao-auto",
	"auto":        "doubao-auto",
	"fast":        "doubao-fast",
	"expert":      "doubao-expert",
}

// BOT_ID 主 bot（docs/03 §3）。
var BOT_ID = envOr("DOUBAO_BOT_ID", "7338286299411103781")

// 风控里不能自动重试的错误码（docs/05 §3）。
var NO_RETRY_CODES = map[int]bool{710022004: true, 710012000: true}
var RETRYABLE_CODES = map[int]bool{710022002: true}

// ── 错误 ──────────────────────────────────────────────────────────────────────

// DoubaoError 上游错误。code 是豆包侧错误码，kind 用于映射 HTTP 状态。
type DoubaoError struct {
	Msg  string
	Code any    // 豆包码可能是 int 或 string；HTTP 状态码分支用 int
	Kind string // auth | risk | rate | upstream | transport
}

func (e *DoubaoError) Error() string { return e.Msg }

func (e *DoubaoError) ToHTTP() int {
	switch e.Kind {
	case "auth":
		return 401
	case "risk":
		return 503
	case "rate":
		return 429
	case "upstream", "transport":
		return 502
	}
	return 500
}

func (e *DoubaoError) Retryable() bool {
	if codeInt, ok := e.Code.(int); ok && NO_RETRY_CODES[codeInt] {
		return false
	}
	return e.Kind == "rate" || codeIn(e.Code, RETRYABLE_CODES)
}

func codeIn(code any, set map[int]bool) bool {
	switch c := code.(type) {
	case int:
		return set[c]
	case float64:
		return set[int(c)]
	}
	return false
}

// classify 错误码/消息 → kind（对齐 doubao._classify）。
func classify(code any, message string) string {
	if codeIn(code, map[int]bool{710012000: true}) ||
		strings.Contains(strings.ToLower(message), "user invalid") {
		return "auth"
	}
	if intCode(code) == 710022004 {
		return "risk"
	}
	if intCode(code) == 710022002 {
		return "rate"
	}
	return "upstream"
}

// intCode 尽力把 any 码转 int（JSON 里可能是 float64 / string）。
func intCode(code any) int {
	switch c := code.(type) {
	case int:
		return c
	case float64:
		return int(c)
	case string:
		if n, err := strconv.Atoi(c); err == nil {
			return n
		}
	}
	return -1
}

// ── cookie / query / 模型 ─────────────────────────────────────────────────────

// BuildCookie 把整串 cookie 裁到最小必需集；缺必需名就直接报错，好过被风控静默拒绝。
func BuildCookie(raw string) (string, error) {
	if raw == "" {
		raw = os.Getenv("DOUBAO_COOKIE")
	}
	pairs := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" || !strings.Contains(part, "=") {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		pairs[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	var missing []string
	for _, k := range REQUIRED_COOKIES {
		if _, ok := pairs[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return "", &DoubaoError{
			Msg:  "cookie 缺少必需项: " + strings.Join(missing, ", "),
			Kind: "auth"}
	}
	var parts []string
	for _, k := range REQUIRED_COOKIES {
		parts = append(parts, k+"="+pairs[k])
	}
	for _, k := range OPTIONAL_COOKIES {
		if v, ok := pairs[k]; ok && v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "; "), nil
}

// BuildQuery 设备面 query 串（对齐 build_query：逐值 quote(v, safe="")）。
func BuildQuery() string {
	var parts []string
	for _, kv := range baseQuery() {
		parts = append(parts, kv[0]+"="+url.QueryEscape(kv[1]))
	}
	return strings.Join(parts, "&")
}

// ResolveModel 返回 (canon, mode)。允许别名与直接传 model_item_key。
func ResolveModel(name string) (string, Mode, error) {
	if name == "" {
		return DEFAULT_MODEL, MODES[DEFAULT_MODEL], nil
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if m, ok := MODES[key]; ok {
		return key, m, nil
	}
	if canon, ok := MODEL_ALIASES[key]; ok {
		return canon, MODES[canon], nil
	}
	// 允许直接传 model_item_key
	for canon, m := range MODES {
		if m.ModelItemKey == name {
			return canon, m, nil
		}
	}
	return "", Mode{}, &DoubaoError{Msg: "unknown model: " + name, Kind: "upstream"}
}

// ── 请求信封 ──────────────────────────────────────────────────────────────────

// BuildEnvelope 五段式信封（docs/03 §4）。所有 ext 值都是字符串。
func BuildEnvelope(text string, mode Mode, conversationID, localConvID, botID string) map[string]any {
	if localConvID == "" {
		localConvID = fmt.Sprintf("local_%d", time.Now().UnixMilli()*1000)
	}
	if botID == "" {
		botID = BOT_ID
	}
	nowMs := time.Now().UnixMilli()
	return map[string]any{
		"client_meta": map[string]any{
			"local_conversation_id": localConvID,
			"conversation_id":       conversationID,
			"bot_id":                botID,
			"last_section_id":       "",
			"last_message_index":    nil,
			"local_permissions": []any{
				map[string]any{"permission_name": "ACCESS_COARSE_LOCATION", "status": 1},
				map[string]any{"permission_name": "ACCESS_FINE_LOCATION", "status": 1},
				map[string]any{"permission_name": "ACCESS_BACKGROUND_LOCATION", "status": 1},
			},
		},
		"messages": []any{map[string]any{
			"local_message_id": uuidV1Like(),
			"content_block": []any{map[string]any{
				"block_type": 10000, // 文本块
				"content": map[string]any{
					"text_block": map[string]any{"text": text, "icon_url": "",
						"icon_url_dark": "", "summary": ""},
					"pc_event_block": "",
				},
				"block_id":      uuidV4(),
				"parent_id":     "",
				"meta_info":     []any{},
				"extra_info":    map[string]any{},
				"append_fields": []any{},
			}},
			"message_status": 0,
		}},
		"option": map[string]any{
			"send_message_scene": "", "create_time_ms": nowMs,
			"collect_id": "", "is_audio": false, "answer_with_suggest": false,
			"agent_mode": mode.AgentMode, "tts_switch": false,
			"need_deep_think": 0, "click_clear_context": false,
			"from_suggest": false, "is_regen": false, "is_replace": false,
			"is_from_click_option": false, "is_from_click_softlink": false,
			"disable_sse_cache": false, "select_text_action": "",
			"is_select_text": false, "resend_for_regen": false, "scene_type": 0,
			"unique_key": uuidV4(), "start_seq": 0,
			"need_create_conversation": conversationID == "",
			"conversation_init_option": map[string]any{"need_ack_conversation": true},
			"conversation_init_ext": map[string]any{
				"model_item_key":   mode.ModelItemKey,
				"reasoning_effort": strconv.Itoa(mode.ReasoningEffort),
				"mode_id":          mode.ModeID,
			},
			"regen_query_id": []any{}, "edit_query_id": []any{}, "regen_instruction": "",
			"no_replace_for_regen": false, "message_from": 0,
			"shared_app_name": "", "shared_app_id": "",
			"sse_recv_event_options":    map[string]any{"support_chunk_delta": true},
			"support_lazy_fetch_stream": true,
			"is_ai_playground":          false, "is_old_user": true,
			"general_task_param": map[string]any{"server_do_compare": true},
			"recovery_option": map[string]any{
				"is_recovery":            false,
				"req_create_time_sec":    nowMs / 1000,
				"append_sse_event_scene": 0},
			"message_storage_type": 0, "related_deleted_message_ids": map[string]any{},
			"connector_info_list": []any{},
			"model_config": map[string]any{
				"model_item_key":     mode.ModelItemKey,
				"model_extra_params": map[string]any{},
				"reasoning_effort":   mode.ReasoningEffort,
			},
			"aggregate_params": map[string]any{
				"mention_skill_list": "[]", "mention_plugin_list": "[]",
				"mention_ext":       "[{}]",
				"conversation_mode": strconv.Itoa(mode.ConversationMode),
				"mode_id":           mode.ModeID,
				"model_item_key":    mode.ModelItemKey,
				"agent_mode":        strconv.Itoa(mode.AgentMode),
				"reasoning_effort":  strconv.Itoa(mode.ReasoningEffort),
				"provider_id":       "",
			},
			"conversation_mode": mode.ConversationMode,
		},
		"user_context": []any{},
		"ext": map[string]any{
			"agent_mode":                    strconv.Itoa(mode.AgentMode),
			"use_deep_think":                "0",
			"sub_conv_firstmet_type":        "1",
			"general_task_param":            string(marshalJSON(map[string]any{"server_do_compare": true})),
			"collection_id":                 "",
			"is_finish":                     "1",
			"conversation_init_option":      string(marshalJSON(map[string]any{"need_ack_conversation": true})),
			"commerce_credit_config_enable": "0",
		},
	}
}

// ── SSE 解析 ──────────────────────────────────────────────────────────────────

// StreamState 单条流的累计状态。
type StreamState struct {
	ConversationID  string
	SectionID       string
	QuestionID      string
	AnswerMessageID string
	LocalMessageID  string
	textParts       []string
	Suggestions     []string
	Brief           string
	BriefSet        bool
	ModelType       string
	LogID           string
	Channels        map[string]bool
}

// Text 拼接出的权威全文（brief 只是预览串，绝不覆盖这里）。
func (st *StreamState) Text() string { return strings.Join(st.textParts, "") }

func newStreamState() *StreamState {
	return &StreamState{Channels: map[string]bool{}}
}

// norm 比较用：折叠所有空白。brief 是预览串会吃换行，只有折叠后才可比。
func norm(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// addText 正文片段直接累加。
//
// 实测两条正文通道（CHUNK_DELTA 与 STREAM_CHUNK patch_object:1）互不重叠，
// 所以不按内容去重（按内容去重会误伤 "哈哈" 这类合法重复片段），末尾再用
// end_type:1 的 brief 做权威校正。tts_content(patch_object 111) 是镜像通道，
// 解析时直接丢弃。
func addText(st *StreamState, piece, channel string) string {
	if piece == "" {
		return ""
	}
	st.Channels[channel] = true
	st.textParts = append(st.textParts, piece)
	return piece
}

// Frame 累加器：逐行喂入，帧完整时返回 (event, data)。
type Frame struct {
	Event string
	Data  map[string]any
	ok    bool
}

func (f *Frame) OK() bool { return f.ok }

// ParseSSELine 把一行喂进 SSE 帧累加器，帧完整时返回 (event, data_obj, true)。
func ParseSSELine(line string, cur map[string]string) (string, map[string]any, bool) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		ev := cur["event"]
		data := cur["data"]
		for k := range cur {
			delete(cur, k)
		}
		if ev == "" {
			return "", nil, false
		}
		obj := map[string]any{}
		if data != "" {
			if err := json.Unmarshal([]byte(data), &obj); err != nil {
				obj = map[string]any{"_raw": data}
			}
		}
		return ev, obj, true
	}
	switch {
	case strings.HasPrefix(line, ":"): // 注释行
	case strings.HasPrefix(line, "event:"):
		cur["event"] = strings.TrimSpace(line[6:])
	case strings.HasPrefix(line, "data:"):
		chunk := strings.TrimSpace(line[5:])
		if prev := cur["data"]; prev != "" {
			cur["data"] = prev + chunk
		} else {
			cur["data"] = chunk
		}
	case strings.HasPrefix(line, "id:"):
		cur["id"] = strings.TrimSpace(line[3:])
	}
	return "", nil, false
}

// ── 规整化事件 ────────────────────────────────────────────────────────────────

// Event Stream 产出的规整化事件（对齐 Python stream() 的元组流）：
// meta（SSE_ACK 之后）、delta（正文片段）、body_end（end_type:1）、
// done（end_type:3）、error（终止）。
type Event struct {
	Kind string // meta | delta | body_end | done | error
	Text string
	Done map[string]any
	Meta *StreamState
	Err  *DoubaoError
}

// handleEvent 处理单个上游 SSE 事件，产出规整化事件（对齐 _handle_event）。
func handleEvent(ev string, data map[string]any, st *StreamState, endStages map[string]bool) ([]Event, *DoubaoError) {
	switch ev {
	case "SSE_HEARTBEAT":
		return nil, nil

	case "SSE_ACK":
		ack := asMap(data["ack_client_meta"])
		if ack != nil {
			if v := asString(ack["conversation_id"]); v != "" {
				st.ConversationID = v
			}
			if v := asString(ack["section_id"]); v != "" {
				st.SectionID = v
			}
		}
		if ql := asList(data["query_list"]); len(ql) > 0 {
			st.QuestionID = asString(asMap(ql[0])["question_id"])
		}
		return []Event{{Kind: "meta", Meta: st}}, nil

	case "FULL_MSG_NOTIFY":
		// 用户自己那条落库；顺带抓服务端真实模型号
		msg := asMap(data["message"])
		if msg != nil {
			if mt := asString(asMap(msg["ext"])["model_type"]); mt != "" {
				st.ModelType = mt
			}
		}
		return nil, nil

	case "STREAM_MSG_NOTIFY":
		content := asMap(data["content"])
		meta := asMap(data["meta"])
		if meta != nil && asString(meta["message_id"]) != "" {
			st.AnswerMessageID = asString(meta["message_id"])
		}
		if content != nil {
			if mt := asString(asMap(content["ext"])["model_type"]); mt != "" {
				st.ModelType = mt
			}
		}
		var out []Event
		for _, blk := range asList(content["content_block"]) {
			t := asString(asMap(asMap(asMap(blk)["content"])["text_block"])["text"])
			if r := addText(st, t, "stream_msg_notify"); r != "" {
				out = append(out, Event{Kind: "delta", Text: r})
			}
		}
		return out, nil

	case "CHUNK_DELTA":
		if r := addText(st, asString(data["text"]), "chunk_delta"); r != "" {
			return []Event{{Kind: "delta", Text: r}}, nil
		}
		return nil, nil

	case "STREAM_CHUNK":
		var out []Event
		for _, op := range asList(data["patch_op"]) {
			opm := asMap(op)
			objF, _ := opm["patch_object"].(float64)
			val := asMap(opm["patch_value"])
			switch int(objF) {
			case 1: // 正文块
				for _, blk := range asList(val["content_block"]) {
					t := asString(asMap(asMap(asMap(blk)["content"])["text_block"])["text"])
					if r := addText(st, t, "stream_chunk_patch1"); r != "" {
						out = append(out, Event{Kind: "delta", Text: r})
					}
				}
			case 50: // ext：建议、is_finish、brief
				ext := asMap(val["ext"])
				if sp := asString(ext["sp_v2"]); sp != "" {
					var arr []map[string]any
					if json.Unmarshal([]byte(sp), &arr) == nil {
						sugg := []string{}
						for _, s := range arr {
							sugg = append(sugg, strings.TrimSpace(asString(s["content"])))
						}
						st.Suggestions = sugg
					}
				}
			case 111: // tts_content 镜像，正文已从别处拿到
			}
		}
		return out, nil

	case "SSE_REPLY_END":
		etF, _ := data["end_type"].(float64)
		switch int(etF) {
		case 1:
			attr := asMap(data["msg_finish_attr"])
			if attr != nil {
				st.Brief = asString(attr["brief"])
				st.BriefSet = true
			}
			endStages["body"] = true
			// 正文此刻已完整：立刻通知下游可以出 finish_reason，
			// 不必等建议阶段（实测可省下 2~3 秒）
			return []Event{{Kind: "body_end", Done: map[string]any{
				"conversation_id":   st.ConversationID,
				"question_id":       st.QuestionID,
				"answer_message_id": st.AnswerMessageID,
				"model_type":        st.ModelType,
				"log_id":            st.LogID,
			}}}, nil
		case 3:
			endStages["final"] = true
			assembled := st.Text()
			if st.BriefSet && norm(st.Brief) != norm(assembled) {
				// brief 是会话列表预览串：实测会吃掉换行、长回答截断。
				// 只作观测，绝不覆盖拼接出的正文。
				slog.Debug("brief differs", "assembled", assembled, "brief", st.Brief)
			}
			channels := make([]string, 0, len(st.Channels))
			for ch := range st.Channels {
				channels = append(channels, ch)
			}
			sort.Strings(channels)
			return []Event{{Kind: "done", Done: map[string]any{
				"text":              assembled,
				"brief":             st.Brief,
				"conversation_id":   st.ConversationID,
				"question_id":       st.QuestionID,
				"answer_message_id": st.AnswerMessageID,
				"model_type":        st.ModelType,
				"suggestions":       st.Suggestions,
				"channels":          channels,
				"log_id":            st.LogID,
			}}}, nil
		}
		return nil, nil

	case "STREAM_ERROR":
		code := data["error_code"]
		msg := orStr(asString(data["error_msg"]), "doubao stream error")
		kind := classify(code, msg)
		detail := msg
		if extra := asMap(data["extra"]); extra != nil {
			if dec := asString(extra["decision"]); dec != "" {
				var d map[string]any
				if json.Unmarshal([]byte(dec), &d) == nil {
					detail = fmt.Sprintf("%s (风控要求验证: type=%s subtype=%s)",
						msg, asString(d["type"]), asString(d["subtype"]))
				}
			}
		}
		return nil, &DoubaoError{
			Msg:  fmt.Sprintf("%s [logid=%s]", detail, st.LogID),
			Code: code, Kind: kind}

	case "gateway-error":
		var code any
		msg := fmt.Sprint(data)
		var inner map[string]any
		if json.Unmarshal([]byte(orStr(asString(data["message"]), "{}")), &inner) == nil && inner != nil {
			code = inner["code"]
			if m := asString(inner["message"]); m != "" {
				msg = m
			}
		}
		return nil, &DoubaoError{
			Msg:  "gateway-error: " + truncate(msg, 300),
			Code: code, Kind: classify(code, msg)}
	}

	slog.Debug("unhandled SSE event", "event", ev)
	return nil, nil
}

// ── 上游客户端 ────────────────────────────────────────────────────────────────

// Client 豆包上游客户端。单账号串行（锁住整条流），天然限频。
type Client struct {
	Cookie  string
	Timeout time.Duration
	Log     *slog.Logger
	// BaseOverride 测试/冒烟注入口：非空时把 https://www.doubao.com 换成假上游
	// 地址。生产路径绝不设置（冒烟用 DOUBAO_UPSTREAM_BASE env，Python 无此入口）。
	BaseOverride string

	HTTP  *http.Client
	mutex sync.Mutex
}

// NewClient 校验 cookie 并装配客户端；cookie 缺必需名时返回错误
// （对齐 DoubaoClient.__init__，调用方据此把 CLIENT 置 nil）。
func NewClient(cookie string, timeout time.Duration, log *slog.Logger) (*Client, error) {
	built, err := BuildCookie(cookie)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	c := &Client{
		Cookie:  built,
		Timeout: timeout,
		Log:     log,
		HTTP: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				ResponseHeaderTimeout: timeout,
			},
		},
	}
	// 冒烟注入口：把上游指到本地黑洞/假上游，绝不打真实豆包。
	if v := os.Getenv("DOUBAO_UPSTREAM_BASE"); v != "" {
		c.BaseOverride = v
	}
	return c, nil
}

func (c *Client) upstreamURL(rawURL string) string {
	if c.BaseOverride == "" {
		return rawURL
	}
	return strings.Replace(rawURL, "https://"+HOST, c.BaseOverride, 1)
}

// Stream 执行一次对话，逐个产出规整化事件。错误以 Kind=="error" 的终止事件
// 传出（对齐 Python 生成器中途 raise 的语义）。整个流期间独占锁：
// 避免同一 cookie 并发触发过频。
func (c *Client) Stream(text, modeName, conversationID string) func(yield func(Event) bool) {
	return func(yield func(Event) bool) {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		for ev := range c.streamLocked(text, modeName, conversationID) {
			if !yield(ev) {
				return
			}
		}
	}
}

func (c *Client) streamLocked(text, modeName, conversationID string) func(yield func(Event) bool) {
	return func(yield func(Event) bool) {
		emit := func(evs []Event, err *DoubaoError) bool {
			if err != nil {
				yield(Event{Kind: "error", Err: err})
				return false
			}
			for _, ev := range evs {
				if !yield(ev) {
					return false
				}
			}
			return true
		}

		canon, mode, err := ResolveModel(modeName)
		if err != nil {
			emit(nil, err.(*DoubaoError))
			return
		}
		_ = canon
		envelope := BuildEnvelope(text, mode, conversationID, "", "")
		payload := marshalJSON(envelope)
		path := COMPLETION_PATH + "?" + BuildQuery()

		headers := map[string]string{
			"content-type":    "application/json",
			"agw-js-conv":     "str", // 抓包是 "str, str"，单值即可（docs/03 §3）
			"user-agent":      USER_AGENT,
			"accept":          "*/*",
			"accept-encoding": "identity", // 免解压，SSE 直读
			"accept-language": "zh-CN,zh;q=0.9",
			"origin":          "https://" + HOST,
			"referer":         "https://" + HOST + "/",
			"cookie":          c.Cookie,
			"priority":        "u=1, i",
		}

		req, err := http.NewRequest("POST", c.upstreamURL("https://"+HOST+path),
			strings.NewReader(string(payload)))
		if err != nil {
			emit(nil, &DoubaoError{Msg: "transport: " + err.Error(), Kind: "transport"})
			return
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			emit(nil, &DoubaoError{Msg: "stream interrupted: " + err.Error(), Kind: "transport"})
			return
		}
		defer resp.Body.Close()

		st := newStreamState()
		st.LocalMessageID = asString(
			asMap(asList(envelope["messages"])[0])["local_message_id"])
		st.LogID = resp.Header.Get("x-tt-logid")

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
			kind := "upstream"
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				kind = "auth"
			}
			emit(nil, &DoubaoError{
				Msg:  fmt.Sprintf("HTTP %d %s", resp.StatusCode, string(body)),
				Code: resp.StatusCode, Kind: kind})
			return
		}

		cur := map[string]string{}
		endStages := map[string]bool{}
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil && line == "" {
				if errors.Is(err, io.EOF) {
					return
				}
				emit(nil, &DoubaoError{
					Msg: "stream interrupted: " + err.Error(), Kind: "transport"})
				return
			}
			ev, data, ok := ParseSSELine(line, cur)
			if !ok {
				if err != nil { // EOF 且无未完成帧
					return
				}
				continue
			}
			out, derr := handleEvent(ev, data, st, endStages)
			if !emit(out, derr) {
				return
			}
			if endStages["final"] {
				return
			}
			if err != nil { // EOF 且无未完成帧
				return
			}
		}
	}
}

// ── 小工具 ────────────────────────────────────────────────────────────────────

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	if l == nil {
		return []any{}
	}
	return l
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func orStr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// marshalJSON 紧凑序列化，不转义 HTML（对齐 json.dumps ensure_ascii=False）。
func marshalJSON(v any) []byte {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte("null")
	}
	return []byte(strings.TrimRight(buf.String(), "\n"))
}

func uuidV4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// uuidV1Like local_message_id（Python uuid1 是时间基 UUID；服务端只当唯一 id，
// Go 侧用 v4 格式等价替代）。
func uuidV1Like() string { return uuidV4() }
