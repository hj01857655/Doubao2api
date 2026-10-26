// Package gateway 对应 Python app/gateway.py — 数据面网关：上游客户端管理、
// 入站鉴权判断、会话缓存、退避重试与 Chat 形状装配。
//
// 协议路由（handler 包）只做协议翻译；凡是"怎么跟豆包打交道"都在这里。
package gateway

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"doubao2api/internal/doubao"
)

// RetryDelay 过频退避的间隔。生产 1s（对齐 Python 硬编码）；测试压到毫秒。
var RetryDelay = time.Second

// Gateway 聚合上游客户端与会话缓存。Client 为 nil 时所有数据面请求以 503 拒绝。
type Gateway struct {
	Client *doubao.Client
	Log    *slog.Logger

	convMu     sync.Mutex
	convByUser map[string]string // key = OpenAI user / metadata.user_id / X-Doubao-Session
}

func New(client *doubao.Client, log *slog.Logger) *Gateway {
	if log == nil {
		log = slog.Default()
	}
	return &Gateway{Client: client, Log: log, convByUser: map[string]string{}}
}

// ReloadClient 按当前环境变量（cookie）重建上游客户端，返回是否可用。
// 扫码登录写回 .env 后走这里热替换，省掉重启。
func (g *Gateway) ReloadClient() bool {
	c, err := doubao.NewClient("", 0, g.Log)
	if err != nil {
		g.Client = nil
		g.Log.Error("重建上游客户端失败：" + err.Error())
		return false
	}
	g.Client = c
	g.Log.Info("上游客户端已按新 cookie 重建")
	return true
}

// ── 鉴权 ──────────────────────────────────────────────────────────────────────

// APIKeyOK 入站鉴权判断：设了 API_KEY 则要求匹配（Authorization: Bearer 或
// x-api-key，后者是 Anthropic SDK 的习惯头）；未设则放行（本地裸奔模式）。
// 响应形状由 handler 按协议决定（OpenAI / Anthropic 两种 401 形状）。
func (g *Gateway) APIKeyOK(r *http.Request) bool {
	key := apiKey()
	if key == "" {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+key ||
		r.Header.Get("x-api-key") == key
}

func apiKey() string {
	if v := os.Getenv("API_KEY"); v != "" {
		return v
	}
	return os.Getenv("DOUBAO_API_KEY")
}

// ── 错误分类 ──────────────────────────────────────────────────────────────────

// ErrType 错误分类 -> OpenAI/通用 err type。
func ErrType(kind string) string {
	switch kind {
	case "auth":
		return "authentication_error"
	case "risk":
		return "risk_control_error"
	case "rate":
		return "rate_limit_error"
	}
	return "server_error"
}

// ── messages -> text ─────────────────────────────────────────────────────────

// Flatten OpenAI content 可能是 str 或 [{type:text,text:..}] 数组。
func Flatten(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, item := range c {
			switch b := item.(type) {
			case string:
				parts = append(parts, b)
			case map[string]any:
				if t, _ := b["type"].(string); t == "" || t == "text" {
					if s, _ := b["text"].(string); s != "" {
						parts = append(parts, s)
					}
				}
			}
		}
		var kept []string
		for _, p := range parts {
			if p != "" {
				kept = append(kept, p)
			}
		}
		return strings.Join(kept, "\n")
	case nil:
		return ""
	default:
		return fmt.Sprint(c)
	}
}

// PickTextAndHistory 取最后一条 user 作为本轮输入，其余作为历史。
func PickTextAndHistory(messages []any) (newest string, history []string, err error) {
	lastUser := -1
	for i, m := range messages {
		if mm, ok := m.(map[string]any); ok && mm["role"] == "user" {
			lastUser = i
		}
	}
	if lastUser < 0 {
		return "", nil, fmt.Errorf("messages 里必须有至少一条 user 消息")
	}
	newestMsg, _ := messages[lastUser].(map[string]any)
	newest = Flatten(newestMsg["content"])
	for i, m := range messages {
		if i == lastUser {
			continue
		}
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		if role == "system" || role == "user" || role == "assistant" {
			if t := Flatten(mm["content"]); t != "" {
				history = append(history, role+": "+t)
			}
		}
	}
	return newest, history, nil
}

// ── conversation cache ───────────────────────────────────────────────────────

// ConvKey 会话缓存 key：OpenAI user / Anthropic metadata.user_id / X-Doubao-Session。
func ConvKey(body map[string]any, header http.Header) string {
	if u, ok := body["user"]; ok && u != nil {
		return fmt.Sprint(u)
	}
	if meta, ok := body["metadata"].(map[string]any); ok {
		if uid := asString(meta["user_id"]); uid != "" {
			return "anth:" + uid
		}
	}
	return header.Get("X-Doubao-Session")
}

// ResolveConversationID 返回 (要用的 conversation_id, 本次用的 key)。
func (g *Gateway) ResolveConversationID(body map[string]any, header http.Header) (string, string) {
	explicit := header.Get("X-Doubao-Conversation-Id")
	if explicit == "" {
		if s, ok := body["conversation_id"].(string); ok {
			explicit = s
		}
	}
	key := ConvKey(body, header)
	if explicit != "" {
		return explicit, key
	}
	if key != "" {
		g.convMu.Lock()
		defer g.convMu.Unlock()
		return g.convByUser[key], key
	}
	return "", key
}

// RememberConversation 会话号回写缓存。
func (g *Gateway) RememberConversation(key, convID string) {
	if key == "" || convID == "" {
		return
	}
	g.convMu.Lock()
	defer g.convMu.Unlock()
	if prev := g.convByUser[key]; prev != convID {
		g.Log.Info("conversation mapped key=" + key + " -> " + convID)
	}
	g.convByUser[key] = convID
}

// ── upstream plumbing ────────────────────────────────────────────────────────

// BackoffStream 过频退避重试（一次）；风控/身份错误立即以 error 事件终止。
//
// 关键约束：已经往下游吐过任何内容就绝不重试。重试会从头再跑一遍上游，
// 已发出的正文会被第二次覆盖式重放，客户端拿到重复文本。
func (g *Gateway) BackoffStream(text, model, convID string) func(yield func(doubao.Event) bool) {
	return func(yield func(doubao.Event) bool) {
		const attempts = 2
		for i := 0; i < attempts; i++ {
			emitted := false
			var failed *doubao.DoubaoError
			for ev := range g.Client.Stream(text, model, convID) {
				if ev.Kind == "error" {
					failed = ev.Err
					break
				}
				emitted = true
				if !yield(ev) {
					return
				}
			}
			if failed == nil {
				return
			}
			if emitted {
				g.Log.Warn(fmt.Sprintf("已下发内容，放弃重试（避免正文重复）kind=%s code=%v",
					failed.Kind, failed.Code))
				yield(doubao.Event{Kind: "error", Err: failed})
				return
			}
			if i+1 < attempts && failed.Retryable() {
				g.Log.Warn(fmt.Sprintf("上游可重试错误(%s code=%v)，退避后重试一次",
					failed.Kind, failed.Code))
				time.Sleep(RetryDelay)
				continue
			}
			yield(doubao.Event{Kind: "error", Err: failed})
			return
		}
	}
}

// Collect 非流式：跑完上游流只取 done 帧。错误事件要带原始 kind/code 透传
// （对齐 Python：stream 抛的 DoubaoError 穿透 collect 直达路由）；
// 只有流正常跑完却没有 done 时才报「上游流未正常结束」。
func (g *Gateway) Collect(text, canon, convID string) (map[string]any, *doubao.DoubaoError) {
	var done map[string]any
	var failed *doubao.DoubaoError
	for ev := range g.BackoffStream(text, canon, convID) {
		switch ev.Kind {
		case "done":
			done = ev.Done
		case "error":
			failed = ev.Err
		}
	}
	if failed != nil {
		return nil, failed
	}
	if done == nil {
		return nil, &doubao.DoubaoError{Msg: "上游流未正常结束", Kind: "upstream"}
	}
	return done, nil
}

// UsageApprox 豆包不回 token 数，给字符数近似（对齐 usage.approx 的 len 语义：
// Python len(str) 是字符数，Go 用 rune 数对齐）。
func UsageApprox(reply, prompt string) map[string]any {
	p := maxInt(1, utf8.RuneCountInString(prompt)/2)
	c := maxInt(1, utf8.RuneCountInString(reply)/2)
	return map[string]any{
		"prompt_tokens":     p,
		"completion_tokens": c,
		"total_tokens":      maxInt(2, (utf8.RuneCountInString(prompt)+utf8.RuneCountInString(reply))/2),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ChatJSON done 帧 -> 最小 Chat Completions JSON（协议桥非流式转换器的输入）。
func ChatJSON(done map[string]any, text, canon string) map[string]any {
	reply, _ := done["text"].(string)
	return map[string]any{
		"id":      "chatcmpl-" + hexID(24),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   canon,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": reply},
			"finish_reason": "stop",
		}},
		"usage": UsageApprox(reply, text),
	}
}

// sseFrame 编一帧 "data: {json}\n\n"。
func sseFrame(obj map[string]any) string {
	return "data: " + string(compactJSON(obj)) + "\n\n"
}

// ChatSSE doubao 流 -> Chat Completions SSE 行流（协议桥流式转换器的输入）。
//
// 两个桥只认 Chat SSE 形状：role/delta 帧、带 usage 的 finish 帧、
// {"error":...} 错误帧、[DONE]。会话号在这里回写缓存。
func (g *Gateway) ChatSSE(text, canon, convID, key string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		rid := "chatcmpl-" + hexID(24)
		created := time.Now().Unix()

		frame := func(obj map[string]any) bool {
			return yield(sseFrame(obj))
		}
		if !frame(map[string]any{
			"id": rid, "object": "chat.completion.chunk",
			"created": created, "model": canon,
			"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"role": "assistant"},
				"finish_reason": nil,
			}},
		}) {
			return
		}
		var done map[string]any
		for ev := range g.BackoffStream(text, canon, convID) {
			switch ev.Kind {
			case "delta":
				if !frame(map[string]any{
					"id": rid, "object": "chat.completion.chunk",
					"created": created, "model": canon,
					"choices": []any{map[string]any{
						"index": 0, "delta": map[string]any{"content": ev.Text},
						"finish_reason": nil,
					}},
				}) {
					return
				}
			case "done":
				done = ev.Done
			case "error":
				g.Log.Warn(fmt.Sprintf("upstream error kind=%s code=%v msg=%s",
					ev.Err.Kind, ev.Err.Code, ev.Err.Msg))
				yield(sseFrame(map[string]any{"error": map[string]any{
					"message": ev.Err.Msg, "type": ErrType(ev.Err.Kind),
					"code": ev.Err.Code}}))
				yield("data: [DONE]\n\n")
				return
			}
		}
		if done != nil {
			convIDOut, _ := done["conversation_id"].(string)
			g.RememberConversation(key, convIDOut)
			reply, _ := done["text"].(string)
			if !frame(map[string]any{
				"id": rid, "object": "chat.completion.chunk",
				"created": created, "model": canon,
				"choices": []any{map[string]any{
					"index": 0, "delta": map[string]any{},
					"finish_reason": "stop",
				}},
				"usage": UsageApprox(reply, text),
			}) {
				return
			}
		}
		yield("data: [DONE]\n\n")
	}
}

// ── 小工具 ────────────────────────────────────────────────────────────────────

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func hexID(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n)
	}
	return hex.EncodeToString(b)[:n]
}

// compactJSON 紧凑序列化，不转义 HTML（对齐 json.dumps ensure_ascii=False,
// separators=(",",":")）。
func compactJSON(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte("null")
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}
