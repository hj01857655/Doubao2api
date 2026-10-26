package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"doubao2api/internal/bridges"
	"doubao2api/internal/config"
	"doubao2api/internal/doubao"
	"doubao2api/internal/gateway"
	"doubao2api/internal/qrlogin"
)

// doubaoMeta done 帧的 doubao 扩展字段（对齐各路由的 "doubao" 键）。
func doubaoMeta(done map[string]any) map[string]any {
	return map[string]any{
		"conversation_id":   done["conversation_id"],
		"question_id":       done["question_id"],
		"answer_message_id": done["answer_message_id"],
		"model_type":        done["model_type"],
		"suggestions":       done["suggestions"],
		"log_id":            done["log_id"],
	}
}

// failOpenAI 上游错误 -> OpenAI 形状错误响应（对齐 openai_chat.fail_from）。
func failOpenAI(w http.ResponseWriter, exc *doubao.DoubaoError) {
	writeJSON(w, exc.ToHTTP(), map[string]any{
		"error": map[string]any{
			"message": exc.Msg, "type": gateway.ErrType(exc.Kind),
			"code": exc.Code, "param": nil}})
}

// ── /v1/chat/completions（对齐 routes/openai_chat.py）────────────────────────

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	if !s.G.APIKeyOK(r) {
		openaiError(w, "invalid api key", "auth_error", "invalid_api_key", http.StatusUnauthorized)
		return
	}
	if s.G.Client == nil {
		openaiError(w, "服务端未配置 DOUBAO_COOKIE", "auth_error", nil, http.StatusServiceUnavailable)
		return
	}
	body := readBody(r)
	messages := anySlice(body["messages"])
	if len(messages) == 0 {
		openaiError(w, "messages 不能为空", "invalid_request_error", nil, http.StatusBadRequest)
		return
	}

	wantStream, _ := body["stream"].(bool)
	modelName, _ := body["model"].(string)
	canon, _, err := doubao.ResolveModel(modelName)
	if err != nil {
		openaiError(w, err.Error(), "invalid_request_error", nil, http.StatusBadRequest)
		return
	}

	newest, history, perr := gateway.PickTextAndHistory(messages)
	if perr != nil {
		openaiError(w, perr.Error(), "invalid_request_error", nil, http.StatusBadRequest)
		return
	}

	convID, key := s.G.ResolveConversationID(body, r.Header)
	// 新会话要把历史带进去，否则上下文丢失
	if convID == "" {
		newest = historyJoined(newest, history)
	}

	rid := "chatcmpl-" + hexID(24)
	created := time.Now().Unix()

	if !wantStream {
		done, derr := s.G.Collect(newest, canon, convID)
		if derr != nil {
			s.Log.Warn("upstream error kind=" + derr.Kind + " code=" + fmtCode(derr.Code) + " msg=" + derr.Msg)
			failOpenAI(w, derr)
			return
		}
		if cid, _ := done["conversation_id"].(string); cid != "" {
			s.G.RememberConversation(key, cid)
		}
		reply, _ := done["text"].(string)
		writeJSON(w, http.StatusOK, map[string]any{
			"id": rid, "object": "chat.completion", "created": created,
			"model": canon,
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": reply},
				"finish_reason": "stop"}},
			"usage":  gateway.UsageApprox(reply, newest),
			"doubao": doubaoMeta(done),
		})
		return
	}

	// 流式：role 首帧 -> delta -> body_end 提前 finish -> done 元数据帧 -> [DONE]。
	// 错误只能以 SSE error 帧收尾（流已开始，HTTP 状态改不了）。
	if convID != "" {
		w.Header().Set("X-Doubao-Conversation-Id", convID)
	}
	writeSSE(w, func(yield func(string) bool) {
		frame := func(obj map[string]any) bool {
			return yield("data: " + string(marshalCompact(obj)) + "\n\n")
		}
		// 首帧：role
		if !frame(map[string]any{
			"id": rid, "object": "chat.completion.chunk",
			"created": created, "model": canon,
			"choices": []any{map[string]any{
				"index": 0, "delta": map[string]any{"role": "assistant"},
				"finish_reason": nil}},
		}) {
			return
		}
		var done map[string]any
		finishedEarly := false
		for ev := range s.G.BackoffStream(newest, canon, convID) {
			switch ev.Kind {
			case "delta":
				if !frame(map[string]any{
					"id": rid, "object": "chat.completion.chunk",
					"created": created, "model": canon,
					"choices": []any{map[string]any{
						"index": 0, "delta": map[string]any{"content": ev.Text},
						"finish_reason": nil}},
				}) {
					return
				}
			case "body_end":
				// 正文已完整，立刻给 finish_reason（实测比 end_type:3 早 2~3 秒）
				finishedEarly = true
				if !frame(map[string]any{
					"id": rid, "object": "chat.completion.chunk",
					"created": created, "model": canon,
					"choices": []any{map[string]any{
						"index": 0, "delta": map[string]any{},
						"finish_reason": "stop"}},
					"doubao": map[string]any{
						"conversation_id": ev.Done["conversation_id"],
						"model_type":      ev.Done["model_type"]},
				}) {
					return
				}
			case "done":
				done = ev.Done
			case "error":
				s.Log.Warn("upstream error kind=" + ev.Err.Kind + " code=" + fmtCode(ev.Err.Code) + " msg=" + ev.Err.Msg)
				frame(map[string]any{"error": map[string]any{
					"message": ev.Err.Msg, "type": gateway.ErrType(ev.Err.Kind),
					"code": ev.Err.Code, "http_status": ev.Err.ToHTTP()}})
				yield("data: [DONE]\n\n")
				return
			}
		}
		if done != nil {
			if cid, _ := done["conversation_id"].(string); cid != "" {
				s.G.RememberConversation(key, cid)
			}
			// 兜底：上游没发 end_type:1 也要给 finish_reason
			if !finishedEarly {
				frame(map[string]any{
					"id": rid, "object": "chat.completion.chunk",
					"created": created, "model": canon,
					"choices": []any{map[string]any{
						"index": 0, "delta": map[string]any{},
						"finish_reason": "stop"}},
				})
			}
			frame(map[string]any{
				"id": rid, "object": "chat.completion.chunk",
				"created": created, "model": canon,
				"choices": []any{},
				"doubao":  doubaoMeta(done),
			})
		}
		yield("data: [DONE]\n\n")
	})
}

// ── /v1/messages（对齐 routes/anthropic_messages.py）─────────────────────────

func (s *Server) anthropic(w http.ResponseWriter, r *http.Request) {
	if !s.G.APIKeyOK(r) {
		writeJSON(w, http.StatusUnauthorized,
			bridges.AnthropicErrorBody("authentication_error", "invalid api key"))
		return
	}
	if s.G.Client == nil {
		writeJSON(w, http.StatusServiceUnavailable,
			bridges.AnthropicErrorBody("authentication_error", "服务端未配置 DOUBAO_COOKIE"))
		return
	}
	body := readBody(r)
	modelName, _ := body["model"].(string)
	canon, _, err := doubao.ResolveModel(modelName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest,
			bridges.AnthropicErrorBody("invalid_request_error", err.Error()))
		return
	}

	chatReq := bridges.AnthropicToChat(body)
	if len(anySlice(chatReq["messages"])) == 0 {
		writeJSON(w, http.StatusBadRequest,
			bridges.AnthropicErrorBody("invalid_request_error", "messages 不能为空"))
		return
	}
	newest, history, perr := gateway.PickTextAndHistory(anySlice(chatReq["messages"]))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest,
			bridges.AnthropicErrorBody("invalid_request_error", perr.Error()))
		return
	}

	convID, key := s.G.ResolveConversationID(body, r.Header)
	if convID == "" {
		newest = historyJoined(newest, history)
	}

	failFrom := func(exc *doubao.DoubaoError) {
		s.Log.Warn("upstream error kind=" + exc.Kind + " code=" + fmtCode(exc.Code) + " msg=" + exc.Msg)
		writeJSON(w, exc.ToHTTP(),
			bridges.AnthropicErrorBody(bridges.AnthropicErrTypeForHTTP(exc.ToHTTP()), exc.Msg))
	}

	wantStream, _ := body["stream"].(bool)
	if !wantStream {
		done, derr := s.G.Collect(newest, canon, convID)
		if derr != nil {
			failFrom(derr)
			return
		}
		if cid, _ := done["conversation_id"].(string); cid != "" {
			s.G.RememberConversation(key, cid)
		}
		out := bridges.ChatJSONToAnthropic(gateway.ChatJSON(done, newest, canon), canon)
		out["doubao"] = map[string]any{
			"conversation_id":   done["conversation_id"],
			"question_id":       done["question_id"],
			"answer_message_id": done["answer_message_id"],
		}
		if cid, _ := done["conversation_id"].(string); cid != "" {
			w.Header().Set("X-Doubao-Conversation-Id", cid)
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	// gateway.ChatSSE 的行流就是 iter.Seq[string]（错误已转 error 帧）
	writeBridgeSSE(w, bridges.ChatSSEToAnthropic(s.G.ChatSSE(newest, canon, convID, key), canon))
}

// ── /v1/responses（对齐 routes/openai_responses.py）──────────────────────────

func (s *Server) responses(w http.ResponseWriter, r *http.Request) {
	if !s.G.APIKeyOK(r) {
		openaiError(w, "invalid api key", "auth_error", "invalid_api_key", http.StatusUnauthorized)
		return
	}
	if s.G.Client == nil {
		writeJSON(w, http.StatusServiceUnavailable,
			bridges.ErrorResponseObj("", "auth_error", "服务端未配置 DOUBAO_COOKIE"))
		return
	}
	body := readBody(r)
	modelName, _ := body["model"].(string)
	canon, _, err := doubao.ResolveModel(modelName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest,
			bridges.ErrorResponseObj(modelName, "invalid_request_error", err.Error()))
		return
	}

	chatReq := bridges.ResponsesToChat(body)
	if len(anySlice(chatReq["messages"])) == 0 {
		writeJSON(w, http.StatusBadRequest,
			bridges.ErrorResponseObj(modelName, "invalid_request_error", "input 不能为空"))
		return
	}
	newest, history, perr := gateway.PickTextAndHistory(anySlice(chatReq["messages"]))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest,
			bridges.ErrorResponseObj(modelName, "invalid_request_error", perr.Error()))
		return
	}

	convID, key := s.G.ResolveConversationID(body, r.Header)
	if convID == "" {
		newest = historyJoined(newest, history)
	}

	failFrom := func(exc *doubao.DoubaoError) {
		s.Log.Warn("upstream error kind=" + exc.Kind + " code=" + fmtCode(exc.Code) + " msg=" + exc.Msg)
		writeJSON(w, exc.ToHTTP(),
			bridges.ErrorResponseObj(canon, exc.Kind, exc.Msg))
	}

	wantStream, _ := body["stream"].(bool)
	if !wantStream {
		done, derr := s.G.Collect(newest, canon, convID)
		if derr != nil {
			failFrom(derr)
			return
		}
		if cid, _ := done["conversation_id"].(string); cid != "" {
			s.G.RememberConversation(key, cid)
		}
		out := bridges.ChatJSONToResponses(gateway.ChatJSON(done, newest, canon), canon)
		if cid, _ := done["conversation_id"].(string); cid != "" {
			w.Header().Set("X-Doubao-Conversation-Id", cid)
		}
		writeJSON(w, http.StatusOK, out)
		return
	}

	// 错误已在 chat_sse 里转 error 帧，桥侧 Err 恒 nil
	writeBridgeSSE(w, bridges.ChatSSEToResponses(bridges.SSELines{
		Seq: s.G.ChatSSE(newest, canon, convID, key),
		Err: func() error { return nil },
	}, canon))
}

// ── /v1/models（对齐 routes/openai_models.py）────────────────────────────────

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	if !s.G.APIKeyOK(r) {
		openaiError(w, "invalid api key", "auth_error", "invalid_api_key", http.StatusUnauthorized)
		return
	}
	// 档位来自本地映射表 doubao.MODES，不查上游——未配置 cookie 时也可用。
	data := make([]any, 0, len(doubao.MODES))
	for name := range doubao.MODES {
		data = append(data, map[string]any{
			"id": name, "object": "model", "owned_by": "doubao", "created": 0})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// ── /health（对齐 create_app 的 health）───────────────────────────────────────

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	models := make([]string, 0, len(doubao.MODES))
	for name := range doubao.MODES {
		models = append(models, name)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       s.G.Client != nil,
		"models":   models,
		"upstream": "https://" + doubao.HOST + doubao.COMPLETION_PATH,
	})
}

// ── /login + /login/qr（对齐 routes/login_qr.py）─────────────────────────────

const loginPageHTML = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>豆包扫码登录 · Doubao2api</title>
<style>
  :root { color-scheme: light dark; }
  body { margin:0; min-height:100vh; display:grid; place-items:center;
         font:15px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif;
         background:#0f1115; color:#e8eaed; }
  .card { width:min(92vw,420px); padding:28px; border-radius:16px; text-align:center;
          background:#171a21; border:1px solid #262b36; }
  h1 { font-size:17px; margin:0 0 4px; font-weight:600; }
  p  { margin:0; color:#9aa4b2; font-size:13px; }
  .qr { width:200px; height:200px; margin:20px auto; padding:10px; border-radius:12px;
        background:#fff; display:grid; place-items:center; }
  .qr img { width:100%; height:100%; display:block; }
  .qr.empty { background:#20242d; color:#9aa4b2; font-size:12px; }
  .state { font-size:14px; min-height:22px; }
  .ok { color:#3ddc84; } .err { color:#ff6b6b; }
  button { margin-top:14px; padding:9px 18px; border-radius:9px; border:1px solid #2f3542;
           background:#222733; color:#e8eaed; font-size:13px; cursor:pointer; }
  code { font-size:12px; color:#9aa4b2; }
</style>
<div class="card">
  <h1>豆包扫码登录</h1>
  <p>用豆包 App 扫一扫，确认后 cookie 自动写入 .env</p>
  <div class="qr empty" id="qr">正在获取二维码…</div>
  <div class="state" id="state"></div>
  <button id="again" hidden>重新出码</button>
  <p style="margin-top:12px"><code id="hint"></code></p>
</div>
<script>
const key = new URLSearchParams(location.search).get("key") || "";
const hdrs = key ? {Authorization: "Bearer " + key} : {};
const qr = document.getElementById("qr"), st = document.getElementById("state");
const again = document.getElementById("again");
document.getElementById("hint").textContent =
  key ? "已带 key 调用 /login/qr" : "未带 key（服务端设了 API_KEY 时需 /login?key=…）";
let timer = null;
let info = null;

function sleep(ms) { return new Promise(r => setTimeout(r, ms)); }

async function pollOnce() {
  const r = await fetch(info.poll, {headers: hdrs});
  const s = await r.json();
  if (!r.ok) throw new Error(s?.error?.message || s?.error || r.status);
  return s;
}

async function run() {
  clearTimeout(timer); again.hidden = true;
  qr.className = "qr empty"; qr.textContent = "正在获取二维码…";
  st.className = "state"; st.textContent = "";
  try {
    const r = await fetch("/login/qr", {method: "POST", headers: hdrs});
    info = await r.json();
    if (!r.ok) throw new Error(info?.error?.message || info?.error || r.status);
  } catch (e) {
    qr.textContent = "出码失败";
    st.className = "state err"; st.textContent = String(e) + "（5 秒后自动重试）";
    timer = setTimeout(run, 5000);
    return;
  }
  qr.className = "qr";
  qr.innerHTML = "";
  const img = new Image(); img.src = info.qrcode_data_uri; img.alt = "登录二维码";
  qr.appendChild(img);
  st.textContent = "等待扫码…";

  const texts = {new: "等待扫码…", scanned: "已扫码，请在手机上确认"};
  for (;;) {
    await sleep(1500);
    let s;
    try {
      s = await pollOnce();
    } catch (e) {
      // 错误停留足够久再自动重试，避免一闪而过看不清
      st.className = "state err"; st.textContent = "轮询失败：" + e + "（5 秒后自动重试）";
      timer = setTimeout(run, 5000);
      return;
    }
    if (s.status === "confirmed") {
      st.className = "state ok";
      st.textContent = "登录成功，cookie 已写入 " + s.cookie_saved +
                       (s.client_ready ? "（上游客户端已就绪）" : "（客户端待重启）");
      qr.className = "qr empty"; qr.textContent = "✔";
      again.hidden = false; return;
    }
    if (s.status === "expired") {
      st.className = "state err"; st.textContent = "二维码已过期，正在自动刷新…";
      timer = setTimeout(run, 800);
      return;
    }
    st.textContent = texts[s.status] || s.description || s.status;
  }
}
again.onclick = run;
run();
</script>
`

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(loginPageHTML))
}

// pruneSessionsLocked 清掉超时会话（调用方需持 sessMu）。
func pruneSessionsLocked(sessions map[string]*qrlogin.Session) {
	now := time.Now()
	for tok, sess := range sessions {
		if now.Sub(sess.CreatedAt) > 10*time.Minute {
			delete(sessions, tok)
		}
	}
}

// startSessionInfo 出码并登记会话（token -> 会话，TTL 10 分钟）。
func (s *Server) startSessionInfo() (map[string]any, error) {
	session := qrlogin.NewSession()
	session.Log = s.Log // 回跳诊断日志跟服务日志同落盘（Default 会写到 stderr 丢失）
	info, err := session.Start()
	if err != nil {
		return nil, err
	}
	token, _ := info["token"].(string)
	s.sessMu.Lock()
	pruneSessionsLocked(s.sessions)
	s.sessions[token] = session
	s.sessMu.Unlock()
	delete(info, "qrcode") // base64 原串没用，data URI 才是给前端的
	info["poll"] = "/login/qr/" + token
	return info, nil
}

func (s *Server) loginQrStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method not allowed"})
		return
	}
	if !s.G.APIKeyOK(r) {
		openaiError(w, "invalid api key", "auth_error", "invalid_api_key", http.StatusUnauthorized)
		return
	}
	info, err := s.startSessionInfo()
	if err != nil {
		openaiError(w, err.Error(), "authentication_error", nil, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) loginQrPoll(w http.ResponseWriter, r *http.Request) {
	if !s.G.APIKeyOK(r) {
		openaiError(w, "invalid api key", "auth_error", "invalid_api_key", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/login/qr/")
	s.sessMu.Lock()
	session := s.sessions[token]
	s.sessMu.Unlock()
	if session == nil {
		writeJSON(w, http.StatusNotFound,
			map[string]any{"error": "会话不存在或已回收，请重新出码"})
		return
	}
	state, err := session.Poll()
	if err != nil {
		openaiError(w, err.Error(), "authentication_error", nil, http.StatusBadGateway)
		return
	}
	if confirmed, _ := state["confirmed"].(bool); !confirmed {
		if state["status"] == "expired" {
			s.sessMu.Lock()
			delete(s.sessions, token)
			s.sessMu.Unlock()
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	// confirmed：收票落盘并热重建上游客户端。cookie 值只进 .env 与环境变量，
	// 不进日志、不进响应体。
	cookie, cerr := session.CollectCookie()
	if cerr != nil {
		s.Log.Error("扫码收票失败: " + cerr.Error())
		openaiError(w, cerr.Error(), "authentication_error", nil, http.StatusBadGateway)
		return
	}
	path, serr := config.SaveCookie(cookie, os.Getenv("DOUBAO_ENV_PATH"))
	if serr != nil {
		s.Log.Error("cookie 写盘失败: " + serr.Error())
		openaiError(w, "cookie 写入失败: "+serr.Error(), "authentication_error", nil, http.StatusBadGateway)
		return
	}
	s.sessMu.Lock()
	delete(s.sessions, token)
	s.sessMu.Unlock()

	ok := s.G.ReloadClient()
	s.Log.Info("扫码登录成功，cookie 已写入 " + path)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "confirmed",
		"confirmed":    true,
		"cookie_saved": path,
		"cookie_names": doubao.REQUIRED_COOKIES,
		"client_ready": ok,
	})
}

// ── 小工具 ────────────────────────────────────────────────────────────────────

func hexID(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n)
	}
	return hex.EncodeToString(b)[:n]
}

func fmtCode(code any) string {
	switch c := code.(type) {
	case nil:
		return "-"
	case float64:
		return fmt.Sprintf("%d", int(c))
	case string:
		return c
	default:
		return fmt.Sprintf("%v", c)
	}
}
