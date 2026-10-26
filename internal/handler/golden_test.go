package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"doubao2api/internal/doubao"
	"doubao2api/internal/gateway"
)

// fakeCookie 10 个必需名的短值假 cookie（<24 字符，避免输出管道脱敏）。
const fakeCookie = "sessionid=fake; sessionid_ss=fake; sid_tt=fake; sid_guard=fake; " +
	"uid_tt=fake; uid_tt_ss=fake; d_ticket=fake; ttwid=fake; s_v_web_id=fake; " +
	"passport_csrf_token=fake"

// captured 假上游捕获到的最近一次请求。
type captured struct {
	mu      sync.Mutex
	Path    string
	Query   string
	Headers http.Header
	Body    []byte
}

func (c *captured) snapshot() captured {
	c.mu.Lock()
	defer c.mu.Unlock()
	return captured{Path: c.Path, Query: c.Query, Headers: c.Headers, Body: c.Body}
}

// fixtureSSE 真实抓包流的逐帧复刻（tests/fixture_capture.txt 同源，正文四段）。
const fixtureSSE = `event: SSE_ACK
data: {"query_list":[{"question_id":"58756254450320130"}],"ack_client_meta":{"conversation_id":"38447554619021570","section_id":"38447554619021826"}}

event: STREAM_MSG_NOTIFY
data: {"content":{"content_block":[{"block_type":10000,"content":{"text_block":{"text":"你"}}}],"ext":{"model_type":"38"}},"meta":{"message_id":"58756254450326274"}}

event: STREAM_CHUNK
data: {"patch_op":[{"patch_object":1,"patch_value":{"content_block":[{"content":{"text_block":{"text":"好"}}}]}}]}

event: STREAM_CHUNK
data: {"patch_op":[{"patch_object":111,"patch_value":{"tts_content":"好"}}]}

event: CHUNK_DELTA
data: {"text":"呀！有什么我可以"}

event: CHUNK_DELTA
data: {"text":"帮你的吗？"}

event: SSE_REPLY_END
data: {"end_type":1,"msg_finish_attr":{"brief":"你好呀！有什么我可以帮你的吗？"}}

event: STREAM_CHUNK
data: {"patch_op":[{"patch_object":50,"patch_value":{"ext":{"sp_v2":"[{\"suggest_type\":1,\"content\":\" 你能做些什么？\"}]"}}}]}

event: SSE_REPLY_END
data: {"end_type":3}

`

// sseWriter 假上游按 SSE 流写响应。
func sseWriter(w http.ResponseWriter, payload string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("x-tt-logid", "LOG1")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, payload)
}

// riskSSE 风控错误流（710022004，不可重试，无正文）。
const riskSSE = "event: STREAM_ERROR\ndata: {\"error_code\":710022004,\"error_msg\":\"rate limited\"}\n\n"

// rateSSE 过频错误流（710022002，可重试，无正文）。
const rateSSE = "event: STREAM_ERROR\ndata: {\"error_code\":710022002,\"error_msg\":\"当前服务访问频繁，请稍后重试\"}\n\n"

// midFailSSE 先吐正文再失败：已发内容绝不重试（否则正文重放）。
const midFailSSE = "event: SSE_ACK\ndata: {\"ack_client_meta\":{\"conversation_id\":\"C1\"}}\n\n" +
	"event: CHUNK_DELTA\ndata: {\"text\":\"前半句\"}\n\n" +
	"event: STREAM_ERROR\ndata: {\"error_code\":710022002,\"error_msg\":\"rate\"}\n\n"

// noBodyEndSSE 只有 end_type:3（上游没发 end_type:1 → 路由要兜底补 finish 帧）。
const noBodyEndSSE = "event: SSE_ACK\ndata: {\"ack_client_meta\":{\"conversation_id\":\"C1\"}}\n\n" +
	"event: CHUNK_DELTA\ndata: {\"text\":\"只有终局\"}\n\n" +
	"event: SSE_REPLY_END\ndata: {\"end_type\":3}\n\n"

type env struct {
	Srv      *Server
	Up       *httptest.Server
	Capture  *captured
	Upstream atomic.Int64 // 上游被调次数
	upFn     func(w http.ResponseWriter, r *http.Request, body []byte)
	upFnMu   sync.Mutex
}

func (e *env) setUpstream(fn func(w http.ResponseWriter, r *http.Request, body []byte)) {
	e.upFnMu.Lock()
	e.upFn = fn
	e.upFnMu.Unlock()
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gateway.RetryDelay = time.Millisecond

	e := &env{Capture: &captured{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completion", func(w http.ResponseWriter, r *http.Request) {
		e.Upstream.Add(1)
		body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		e.Capture.mu.Lock()
		e.Capture.Path = r.URL.Path
		e.Capture.Query = r.URL.RawQuery
		e.Capture.Headers = r.Header.Clone()
		e.Capture.Body = body
		e.Capture.mu.Unlock()
		e.upFnMu.Lock()
		fn := e.upFn
		e.upFnMu.Unlock()
		if fn == nil {
			sseWriter(w, fixtureSSE) // 默认：真实抓包流
			return
		}
		fn(w, r, body)
	})
	up := httptest.NewServer(mux)
	t.Cleanup(up.Close)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client, err := doubao.NewClient(fakeCookie, 5*time.Second, log)
	if err != nil {
		t.Fatal(err)
	}
	client.BaseOverride = up.URL
	e.Srv = NewServer(gateway.New(client, log), log)
	e.Up = up
	return e
}

func postJSON(t *testing.T, srv *Server, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, srv *Server, path string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// sseDataFrames 拆出全部 "data: " 帧（含 [DONE]）。
func sseDataFrames(body string) []string {
	var out []string
	for _, block := range strings.Split(body, "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data: ") {
				out = append(out, strings.TrimPrefix(line, "data: "))
			}
		}
	}
	return out
}

// deltaJoin 流式帧里 choices[].delta.content 的拼接。
func deltaJoin(t *testing.T, frames []string) string {
	t.Helper()
	var joined strings.Builder
	for _, f := range frames {
		var obj map[string]any
		if json.Unmarshal([]byte(f), &obj) != nil {
			continue
		}
		chs, _ := obj["choices"].([]any)
		if len(chs) != 1 {
			continue
		}
		delta, _ := chs[0].(map[string]any)["delta"].(map[string]any)
		if c, ok := delta["content"].(string); ok {
			joined.WriteString(c)
		}
	}
	return joined.String()
}

func mustJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("响应不是 JSON: %v\n%s", err, raw)
	}
	return m
}

// ── 基础面 ────────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	e := newEnv(t)
	rec := get(t, e.Srv, "/health", nil)
	if rec.Code != 200 {
		t.Fatalf("health status = %d", rec.Code)
	}
	m := mustJSON(t, rec.Body.String())
	if m["ok"] != true {
		t.Errorf("ok = %v", m["ok"])
	}
	if upstream, _ := m["upstream"].(string); upstream != "https://www.doubao.com/chat/completion" {
		t.Errorf("upstream = %v", m["upstream"])
	}
	models, _ := m["models"].([]any)
	if len(models) != 6 {
		t.Errorf("models 数 = %d", len(models))
	}
}

func TestAPIKeyGuard(t *testing.T) {
	e := newEnv(t)
	t.Setenv("API_KEY", "smoke-key")

	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 401 {
		t.Fatalf("chat 401 期待, got %d", rec.Code)
	}
	m := mustJSON(t, rec.Body.String())
	if m["error"].(map[string]any)["code"] != "invalid_api_key" {
		t.Errorf("openai 401 形状: %v", m)
	}

	rec = postJSON(t, e.Srv, "/v1/messages",
		`{"model":"doubao-fast","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 401 {
		t.Fatalf("anthropic 401 期待, got %d", rec.Code)
	}
	m = mustJSON(t, rec.Body.String())
	if m["error"].(map[string]any)["type"] != "authentication_error" {
		t.Errorf("anthropic 401 形状: %v", m)
	}

	// Bearer 正确则放行
	rec = postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"Authorization": "Bearer smoke-key"})
	if rec.Code != 200 {
		t.Fatalf("带正确 key 应 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCookieMissing503(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(gateway.New(nil, log), log) // CLIENT 未就绪

	rec := postJSON(t, srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 503 {
		t.Fatalf("chat 503 期待, got %d", rec.Code)
	}
	rec = postJSON(t, srv, "/v1/messages",
		`{"model":"doubao-fast","messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 503 {
		t.Fatalf("messages 503 期待, got %d", rec.Code)
	}
	m := mustJSON(t, rec.Body.String())
	if m["error"].(map[string]any)["type"] != "authentication_error" {
		t.Errorf("anthropic 503 形状: %v", m)
	}
	rec = postJSON(t, srv, "/v1/responses",
		`{"model":"doubao-fast","input":"hi"}`, nil)
	if rec.Code != 503 {
		t.Fatalf("responses 503 期待, got %d", rec.Code)
	}
}

func TestChatBodyValidation(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/chat/completions", `{"messages":[]}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "messages 不能为空") {
		t.Errorf("空 messages 400: %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, e.Srv, "/v1/chat/completions",
		`{"model":"no-such","messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "unknown model") {
		t.Errorf("未知模型 400: %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"system","content":"only"}]}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "至少一条 user") {
		t.Errorf("无 user 消息 400: %d %s", rec.Code, rec.Body.String())
	}
}

// ── 上游请求面（docs/03 契约）────────────────────────────────────────────────

func TestUpstreamRequestShape(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"model":"doubao-pro","messages":[{"role":"user","content":"你好"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cap := e.Capture.snapshot()
	if cap.Path != "/chat/completion" {
		t.Errorf("path = %s", cap.Path)
	}
	q := cap.Query
	for _, must := range []string{"aid=582478", "fp=verify_", "tea_uuid=", "web_tab_id=",
		"chromium_version=", "client_platform=pc_client"} {
		if !strings.Contains(q, must) {
			t.Errorf("query 缺 %s: %s", must, q)
		}
	}
	if strings.Contains(q, "a_bogus") || strings.Contains(q, "msToken") {
		t.Error("query 不应含签名参数（实测不校验）")
	}
	if got := cap.Headers.Get("Agw-Js-Conv"); got != "str" {
		t.Errorf("agw-js-conv = %q", got)
	}
	if got := cap.Headers.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("accept-encoding = %q（SSE 直读免解压）", got)
	}
	if !strings.Contains(cap.Headers.Get("User-Agent"), "SamanthaDoubao/") {
		t.Errorf("UA = %q", cap.Headers.Get("User-Agent"))
	}
	cookie := cap.Headers.Get("Cookie")
	if strings.Count(cookie, "=") != 10 {
		t.Errorf("cookie 应裁到 10 名: %q", cookie)
	}
	var envelope map[string]any
	if err := json.Unmarshal(cap.Body, &envelope); err != nil {
		t.Fatalf("envelope 不是 JSON: %v", err)
	}
	opt := envelope["option"].(map[string]any)
	if opt["need_create_conversation"] != true {
		t.Error("首轮 need_create_conversation=true")
	}
	initExt := opt["conversation_init_ext"].(map[string]any)
	if initExt["model_item_key"] != "5" {
		t.Errorf("doubao-pro -> model_item_key=5, got %v", initExt["model_item_key"])
	}
}

// ── chat 非流式 ───────────────────────────────────────────────────────────────

func TestChatNonStream(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"model":"doubao-fast","messages":[{"role":"user","content":"你好"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	m := mustJSON(t, rec.Body.String())
	msg := m["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "你好呀！有什么我可以帮你的吗？" {
		t.Errorf("content = %v", msg["content"])
	}
	db := m["doubao"].(map[string]any)
	if db["conversation_id"] != "38447554619021570" {
		t.Errorf("doubao.conversation_id = %v", db["conversation_id"])
	}
	if db["model_type"] != "38" {
		t.Errorf("doubao.model_type = %v", db["model_type"])
	}
	usage := m["usage"].(map[string]any)
	if usage["total_tokens"].(float64) < 2 {
		t.Errorf("usage 估算 = %v", usage)
	}
}

// ── chat 流式 ─────────────────────────────────────────────────────────────────

func TestChatStream(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"model":"doubao-fast","stream":true,"messages":[{"role":"user","content":"你好"}]}`,
		map[string]string{"X-Doubao-Session": "sess-A"})
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	frames := sseDataFrames(rec.Body.String())
	if len(frames) == 0 || frames[len(frames)-1] != "[DONE]" {
		t.Fatalf("末帧应是 [DONE]: %v", frames)
	}
	// 首帧 role
	first := mustJSON(t, frames[0])
	d0 := first["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	if d0["role"] != "assistant" {
		t.Errorf("首帧 delta.role = %v", d0["role"])
	}
	// delta 拼接 = 权威全文（两条通道不重叠，tts 镜像 111 丢弃）
	if got := deltaJoin(t, frames); got != "你好呀！有什么我可以帮你的吗？" {
		t.Errorf("delta 拼接 = %q", got)
	}
	// body_end 提前 finish（带 doubao 扩展，早 end_type:3 约 2~3 秒）
	// + done 元数据帧（choices:[]，六字段）
	foundEarlyFinish, foundMeta := false, false
	for _, f := range frames {
		var obj map[string]any
		if json.Unmarshal([]byte(f), &obj) != nil {
			continue
		}
		chs, _ := obj["choices"].([]any)
		db, hasDB := obj["doubao"].(map[string]any)
		if hasDB && len(chs) == 1 && chs[0].(map[string]any)["finish_reason"] == "stop" {
			foundEarlyFinish = true
			if db["conversation_id"] != "38447554619021570" {
				t.Errorf("提前 finish 帧 doubao.conversation_id = %v", db["conversation_id"])
			}
		}
		if hasDB && len(chs) == 0 {
			foundMeta = true
			if db["answer_message_id"] != "58756254450326274" {
				t.Errorf("元数据帧 answer_message_id = %v", db["answer_message_id"])
			}
			if db["model_type"] != "38" {
				t.Errorf("元数据帧 model_type = %v", db["model_type"])
			}
		}
	}
	if !foundEarlyFinish {
		t.Error("应有 body_end 提前 finish 帧")
	}
	if !foundMeta {
		t.Error("应有 done 元数据帧（choices:[]）")
	}
	if n := strings.Count(rec.Body.String(), "[DONE]"); n != 1 {
		t.Errorf("[DONE] 应恰一次, got %d", n)
	}
}

func TestChatStreamNoBodyEndFallback(t *testing.T) {
	e := newEnv(t)
	e.setUpstream(func(w http.ResponseWriter, r *http.Request, body []byte) {
		sseWriter(w, noBodyEndSSE)
	})
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	// 上游没发 end_type:1 也要给 finish_reason
	frames := sseDataFrames(rec.Body.String())
	found := false
	for _, f := range frames {
		var obj map[string]any
		if json.Unmarshal([]byte(f), &obj) != nil {
			continue
		}
		if chs, ok := obj["choices"].([]any); ok && len(chs) == 1 {
			if chs[0].(map[string]any)["finish_reason"] == "stop" {
				found = true
			}
		}
	}
	if !found {
		t.Error("无 body_end 时应兜底补 finish 帧")
	}
}

func TestChatStreamErrorFrame(t *testing.T) {
	e := newEnv(t)
	e.setUpstream(func(w http.ResponseWriter, r *http.Request, body []byte) {
		sseWriter(w, riskSSE)
	})
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("流式请求应 200（流已开始，HTTP 状态改不了）, got %d", rec.Code)
	}
	var errFrame map[string]any
	for _, f := range sseDataFrames(rec.Body.String()) {
		var obj map[string]any
		if json.Unmarshal([]byte(f), &obj) == nil {
			if err, ok := obj["error"].(map[string]any); ok {
				errFrame = err
			}
		}
	}
	if errFrame == nil {
		t.Fatal("应有 SSE error 帧")
	}
	if errFrame["type"] != "risk_control_error" {
		t.Errorf("风控 err type = %v", errFrame["type"])
	}
	if errFrame["http_status"] != float64(503) {
		t.Errorf("风控 http_status = %v", errFrame["http_status"])
	}
	if !strings.HasSuffix(strings.TrimSpace(rec.Body.String()), "[DONE]") {
		t.Error("错误后仍以 [DONE] 收尾")
	}
}

// ── 退避重试（已发内容绝不重试）──────────────────────────────────────────────

func TestBackoffRetryNoEmit(t *testing.T) {
	e := newEnv(t)
	var calls atomic.Int64
	e.setUpstream(func(w http.ResponseWriter, r *http.Request, body []byte) {
		if calls.Add(1) == 1 {
			sseWriter(w, rateSSE) // 可重试且未吐正文
			return
		}
		sseWriter(w, fixtureSSE)
	})
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("重试后应成功: %d %s", rec.Code, rec.Body.String())
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("无正文失败应重试一次, 上游被调 %d 次", n)
	}
}

func TestNoRetryAfterEmit(t *testing.T) {
	e := newEnv(t)
	e.setUpstream(func(w http.ResponseWriter, r *http.Request, body []byte) {
		sseWriter(w, midFailSSE) // 已吐 "前半句" 后失败
	})
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"stream":true,"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if n := e.Upstream.Load(); n != 1 {
		t.Errorf("已发正文绝不重试, 上游被调 %d 次", n)
	}
	if got := strings.Count(rec.Body.String(), "前半句"); got != 1 {
		t.Errorf("正文不得重复, 出现 %d 次", got)
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Error("失败应以 error 帧呈现")
	}
}

// ── 会话缓存 ──────────────────────────────────────────────────────────────────

func TestConversationCacheAndHeader(t *testing.T) {
	e := newEnv(t)
	// 显式 X-Doubao-Conversation-Id：进 envelope
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`,
		map[string]string{"X-Doubao-Conversation-Id": "C-EXIST"})
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var envelope map[string]any
	cap := e.Capture.snapshot()
	json.Unmarshal(cap.Body, &envelope)
	if envelope["client_meta"].(map[string]any)["conversation_id"] != "C-EXIST" {
		t.Error("显式会话号应进 envelope")
	}

	// user 字段作缓存 key：第二轮不显式传 → 复用首轮回写的会话号
	rec = postJSON(t, e.Srv, "/v1/chat/completions",
		`{"user":"u1","messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("首轮 status=%d", rec.Code)
	}
	rec = postJSON(t, e.Srv, "/v1/chat/completions",
		`{"user":"u1","messages":[{"role":"user","content":"再来"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("次轮 status=%d", rec.Code)
	}
	cap = e.Capture.snapshot()
	envelope = nil
	json.Unmarshal(cap.Body, &envelope)
	if got := envelope["client_meta"].(map[string]any)["conversation_id"]; got != "38447554619021570" {
		t.Errorf("次轮应复用缓存会话号, got %v", got)
	}
}

// ── Anthropic 路由 ────────────────────────────────────────────────────────────

func TestAnthropicNonStream(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/messages",
		`{"model":"doubao-fast","max_tokens":100,"messages":[{"role":"user","content":"你好"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	m := mustJSON(t, rec.Body.String())
	if m["type"] != "message" || m["role"] != "assistant" {
		t.Errorf("message 形状: %v / %v", m["type"], m["role"])
	}
	if m["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v", m["stop_reason"])
	}
	usage := m["usage"].(map[string]any)
	for _, k := range []string{"input_tokens", "output_tokens",
		"cache_creation_input_tokens", "cache_read_input_tokens"} {
		if _, ok := usage[k]; !ok {
			t.Errorf("usage 缺 %s", k)
		}
	}
	if _, ok := m["doubao"]; !ok {
		t.Error("应有 doubao 扩展")
	}
	if rec.Header().Get("X-Doubao-Conversation-Id") != "38447554619021570" {
		t.Errorf("X-Doubao-Conversation-Id = %q", rec.Header().Get("X-Doubao-Conversation-Id"))
	}
}

func TestAnthropicStream(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/messages",
		`{"model":"doubao-fast","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"你好"}]}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		// 正文按上游四段 delta 逐帧下发，整句连续串不存在
		`"text":"你"`, `"text":"好"`, `"text":"呀！有什么我可以"`, `"text":"帮你的吗？"`,
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Anthropic SSE 缺 %q", want)
		}
	}
}

func TestAnthropicValidation(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/messages", `{"model":"no-such"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "unknown model") {
		t.Errorf("未知模型 400: %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, e.Srv, "/v1/messages", `{"model":"doubao-fast"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "messages 不能为空") {
		t.Errorf("空 messages 400: %d %s", rec.Code, rec.Body.String())
	}
}

// ── Responses 路由 ────────────────────────────────────────────────────────────

func TestResponsesNonStream(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/responses",
		`{"model":"doubao-fast","input":"你好"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	m := mustJSON(t, rec.Body.String())
	if m["object"] != "response" || m["status"] != "completed" {
		t.Errorf("response 形状: %v / %v", m["object"], m["status"])
	}
	output, _ := m["output"].([]any)
	if len(output) == 0 {
		t.Fatal("output 不应为空")
	}
	if item := output[0].(map[string]any); item["type"] != "message" {
		t.Errorf("output[0].type = %v", item["type"])
	}
	usage := m["usage"].(map[string]any)
	for _, k := range []string{"input_tokens", "input_tokens_details",
		"output_tokens", "output_tokens_details", "total_tokens"} {
		if _, ok := usage[k]; !ok {
			t.Errorf("Responses usage 缺 %s（五字段必填）", k)
		}
	}
}

func TestResponsesValidation(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/responses", `{"model":"doubao-fast"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "input 不能为空") {
		t.Errorf("空 input 400: %d %s", rec.Code, rec.Body.String())
	}
	m := mustJSON(t, rec.Body.String())
	if m["status"] != "failed" {
		t.Errorf("错误也应是 failed Response 对象: %v", m["status"])
	}
}

func TestResponsesStream(t *testing.T) {
	e := newEnv(t)
	rec := postJSON(t, e.Srv, "/v1/responses",
		`{"model":"doubao-fast","stream":true,"input":"你好"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"type":"response.created"`,
		`"type":"response.in_progress"`,
		`"type":"response.output_item.added"`,
		`"type":"response.output_text.delta"`,
		"你好呀！有什么我可以帮你的吗？",
		`"type":"response.output_item.done"`,
		`"type":"response.completed"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Responses SSE 缺 %q", want)
		}
	}
	if strings.Contains(body, "[DONE]") {
		t.Error("Responses 协议没有 [DONE]")
	}
}

// ── models ────────────────────────────────────────────────────────────────────

func TestModels(t *testing.T) {
	e := newEnv(t)
	rec := get(t, e.Srv, "/v1/models", nil)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	m := mustJSON(t, rec.Body.String())
	data, _ := m["data"].([]any)
	if len(data) != 6 {
		t.Errorf("data 数 = %d", len(data))
	}
	first := data[0].(map[string]any)
	if first["object"] != "model" || first["owned_by"] != "doubao" {
		t.Errorf("model 条目形状: %v", first)
	}
}

// ── 上游 HTTP 非 200 / transport ──────────────────────────────────────────────

func TestUpstreamHTTPError(t *testing.T) {
	e := newEnv(t)
	e.setUpstream(func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"code":"auth"}`)
	})
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 401 {
		t.Errorf("上游 403 -> auth 401, got %d", rec.Code)
	}
}

func TestUpstreamTransportError(t *testing.T) {
	e := newEnv(t)
	e.Srv.G.Client.BaseOverride = "http://127.0.0.1:9" // 黑洞，绝不打真实上游
	rec := postJSON(t, e.Srv, "/v1/chat/completions",
		`{"messages":[{"role":"user","content":"hi"}]}`, nil)
	if rec.Code != 502 {
		t.Errorf("transport 错误 -> 502, got %d", rec.Code)
	}
}
