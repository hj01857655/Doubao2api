// Package handler 对应 Python app/__init__.py 的 create_app + app/routes/*：
// 路由、鉴权、协议形状输出。数据面（上游/重试/会话缓存）在 gateway 包，
// 方言翻译在 bridges 包，扫码登录状态机在 qrlogin 包。
package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"doubao2api/internal/gateway"
	"doubao2api/internal/qrlogin"
)

// Server 聚合全部依赖；Handler 挂到 http.ServeMux。
type Server struct {
	G   *gateway.Gateway
	Log *slog.Logger

	// 扫码会话表：扫码期间才存在，确认/过期/超时即回收，进程重启即清空。
	sessMu   sync.Mutex
	sessions map[string]*qrlogin.Session
}

func NewServer(g *gateway.Gateway, log *slog.Logger) *Server {
	return &Server{G: g, Log: log, sessions: map[string]*qrlogin.Session{}}
}

// Handler 组装路由。Python 无全局 before_request：guard 由各路由自带；
// /health 与 /login 页面不鉴权。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/login", s.loginPage)
	mux.HandleFunc("/login/qr", s.loginQrStart)
	mux.HandleFunc("/login/qr/", s.loginQrPoll)
	mux.HandleFunc("/v1/chat/completions", s.chat)
	mux.HandleFunc("/v1/messages", s.anthropic)
	mux.HandleFunc("/v1/responses", s.responses)
	mux.HandleFunc("/v1/models", s.models)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "Not Found: " + r.URL.Path, "type": "not_found_error"}})
	})
	return mux
}

// ── 响应辅助 ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(marshalCompact(v))
}

// openaiError 对齐 gateway.openai_error：{"error":{message,type,code,param}}。
func openaiError(w http.ResponseWriter, message, errType string, code any, status int) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message, "type": errType, "code": code, "param": nil}})
}

func marshalCompact(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte("null")
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// sseHeaders 流式响应头（对齐三条流式路由的 headers）。
func sseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
}

// writeSSE 把行流写给客户端（行已是完整 "data: ...\n\n"）。
func writeSSE(w http.ResponseWriter, lines func(yield func(string) bool)) {
	sseHeaders(w)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for line := range lines {
		if _, err := io.WriteString(w, line); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// writeBridgeSSE 把桥产出的完整帧（[]byte）写给客户端（Anthropic/Responses 流）。
func writeBridgeSSE(w http.ResponseWriter, frames func(yield func([]byte) bool)) {
	sseHeaders(w)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for frame := range frames {
		if _, err := w.Write(frame); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func readBody(r *http.Request) map[string]any {
	var m map[string]any
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		_ = json.Unmarshal(raw, &m)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func anySlice(v any) []any {
	l, _ := v.([]any)
	return l
}

// historyJoined 新会话要把历史带进去，否则上下文丢失。
func historyJoined(text string, history []string) string {
	if len(history) == 0 {
		return text
	}
	return strings.Join(history, "\n") + "\n\nuser: " + text
}
