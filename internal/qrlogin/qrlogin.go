// Package qrlogin 对应 Python app/qrlogin.py — 豆包扫码登录（passport 三端点）。
//
// 端点、参数与坑都是实测结论（2026-10-17 curl 直连 www.doubao.com），勿凭感觉改：
//
//	GET /passport/safe/csrf_token/?aid=497858
//	GET /passport/web/get_qrcode/?next=<urlencoded>&aid=497858  + 头 x-tt-passport-csrf-token
//	GET /passport/web/check_qrconnect/?next=...&token=...&aid=497858  + 同一个 CSRF 头
//
// 三个坑（都实测踩过）：
//
//  1. 登录面 aid = 497858，与桌面 chat 面 aid = 582478 不是一套。串用只会拿到
//     error_code 22「非法应用」。
//  2. 端点是 GET + 尾斜杠，且必须带 next（登录后回跳地址）。
//  3. CSRF 必须放在头 x-tt-passport-csrf-token，塞进 query 不生效。
package qrlogin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"doubao2api/internal/doubao"
)

const (
	LoginAID     = "497858"
	loginReferer = "https://www.doubao.com/chat/login"
	// NEXT_URL 扫码成功后回跳目标：与登录页同源，服务端据此决定 redirect_url 的落点。
	NextURL = "https://www.doubao.com"
	// MaxHops 跟随 redirect_url 的最大跳数：实测登录回跳链为 1~3 跳，6 跳足够且能防打转。
	MaxHops = 6
)

// Session 一次扫码登录的状态机：Start() 出码 → Poll() 轮询 → CollectCookie() 收票。
type Session struct {
	Base string // 官方 https://www.doubao.com；测试注入 httptest 地址
	AID  string
	Next string

	HTTP *http.Client
	Log  *slog.Logger

	mu             sync.Mutex
	cookies        map[string]string
	token          string
	qrcode         string
	qrcodeIndexURL string
	expireTime     int64
	Status         string
	Description    string
	RedirectURL    string
	CreatedAt      time.Time
}

// NewSession 建一次扫码会话（Base 空时用官方域）。
func NewSession() *Session {
	return &Session{
		Base: "https://www.doubao.com",
		AID:  LoginAID,
		Next: NextURL,
		Log:  slog.Default(),
		HTTP: &http.Client{
			Transport: &http.Transport{
				// 对齐 Python http.client：无代理直连、HTTP/1.1（自定义 Transport
				// 默认不启用 H2）。系统代理/H2 下的回跳链行为未实测过，不作数。
				Proxy: nil,
			},
			// 不自动跟随跳转：登录回跳链上的每一跳都可能下发 Set-Cookie，必须逐跳收。
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cookies:   map[string]string{},
		CreatedAt: time.Now(),
	}
}

func (s *Session) headers(csrf string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", doubao.USER_AGENT)
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Referer", loginReferer)
	h.Set("Origin", s.Base)
	if csrf != "" {
		h.Set("x-tt-passport-csrf-token", csrf)
	}
	return h
}

// httpGet GET 一次，返回 (status, body, location)。不自动跟随跳转。
func (s *Session) httpGet(rawURL string, hdr http.Header) (int, []byte, string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header = hdr.Clone()
	s.mu.Lock()
	var pairs []string
	for k, v := range s.cookies {
		pairs = append(pairs, k+"="+v)
	}
	s.mu.Unlock()
	if len(pairs) > 0 {
		req.Header.Set("Cookie", strings.Join(pairs, "; "))
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	s.mergeCookies(resp)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, resp.Header.Get("Location"), nil
}

func (s *Session) mergeCookies(resp *http.Response) {
	for _, c := range resp.Cookies() {
		s.mu.Lock()
		s.cookies[c.Name] = c.Value
		s.mu.Unlock()
	}
}

func jsonBody(body []byte) map[string]any {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func fail(action string, data map[string]any, body []byte) *doubao.DoubaoError {
	code := -1
	if data != nil {
		if f, ok := data["error_code"].(float64); ok {
			code = int(f)
		}
	}
	desc := ""
	if data != nil {
		desc = asString(data["description"])
	}
	return &doubao.DoubaoError{
		Msg:  fmt.Sprintf("%s失败: error_code=%d %s %s", action, code, desc, truncate(string(body), 120)),
		Kind: "auth"}
}

// PNGDataURI get_qrcode 的 qrcode 字段是裸 base64 PNG（不是 data URI）。
func PNGDataURI(qrcodeB64 string) string {
	if strings.HasPrefix(qrcodeB64, "data:") {
		return qrcodeB64
	}
	return "data:image/png;base64," + qrcodeB64
}

// CsrfToken 取（并缓存）passport CSRF。响应里没有 token 就是被风控拦了。
func (s *Session) CsrfToken() (string, error) {
	s.mu.Lock()
	token := s.cookies["passport_csrf_token"]
	s.mu.Unlock()
	if token != "" {
		return token, nil
	}
	status, body, _, err := s.httpGet(
		s.Base+"/passport/safe/csrf_token/?aid="+s.AID, s.headers(""))
	if err != nil {
		return "", fail("获取 CSRF", nil, []byte(err.Error()))
	}
	_ = status
	data := jsonBody(body)
	inner, _ := data["data"].(map[string]any)
	token = asString(mapOrEmpty(inner)["passport_csrf_token"])
	if token == "" {
		if inner == nil {
			inner = data
		}
		return "", fail("获取 CSRF", inner, body)
	}
	s.mu.Lock()
	s.cookies["passport_csrf_token"] = token
	s.mu.Unlock()
	return token, nil
}

// Start 取二维码。next 必须带，否则服务端按非法调用处置。
func (s *Session) Start() (map[string]any, error) {
	csrf, err := s.CsrfToken()
	if err != nil {
		return nil, err
	}
	rawURL := fmt.Sprintf("%s/passport/web/get_qrcode/?next=%s&aid=%s",
		s.Base, url.QueryEscape(s.Next), s.AID)
	_, body, _, err := s.httpGet(rawURL, s.headers(csrf))
	if err != nil {
		return nil, fail("获取二维码", nil, []byte(err.Error()))
	}
	data := jsonBody(body)
	d := mapOrEmpty(asMapOrEmpty(data["data"]))
	errCode := -1
	if f, ok := d["error_code"].(float64); ok {
		errCode = int(f)
	} else if f, ok := data["error_code"].(float64); ok {
		errCode = int(f)
	}
	token := asString(d["token"])
	if errCode != 0 || token == "" {
		return nil, fail("获取二维码", d, body)
	}
	s.mu.Lock()
	s.token = token
	s.qrcode = asString(d["qrcode"])
	s.qrcodeIndexURL = asString(d["qrcode_index_url"])
	if f, ok := d["expire_time"].(float64); ok {
		s.expireTime = int64(f)
	}
	s.Status = "new"
	s.mu.Unlock()
	info := map[string]any{
		"token":       token,
		"qrcode":      s.qrcode,
		"status":      s.Status,
		"expire_time": s.expireTime,
		"app_name":    d["app_name"],
		"web_name":    d["web_name"],
	}
	if s.qrcode != "" {
		info["qrcode_data_uri"] = PNGDataURI(s.qrcode)
	} else {
		info["qrcode_data_uri"] = ""
	}
	if s.qrcodeIndexURL != "" {
		info["qrcode_index_url"] = s.qrcodeIndexURL
	}
	return info, nil
}

// Poll 查一次扫码状态；服务端不区分「过期」错误码，靠 description 判定。
func (s *Session) Poll() (map[string]any, error) {
	s.mu.Lock()
	token := s.token
	csrf := s.cookies["passport_csrf_token"]
	s.mu.Unlock()
	if token == "" {
		return nil, &doubao.DoubaoError{Msg: "尚未 Start()，没有二维码 token", Kind: "auth"}
	}
	rawURL := fmt.Sprintf("%s/passport/web/check_qrconnect/?next=%s&token=%s&aid=%s",
		s.Base, url.QueryEscape(s.Next), token, s.AID)
	_, body, _, err := s.httpGet(rawURL, s.headers(csrf))
	if err != nil {
		return nil, fail("查询扫码状态", nil, []byte(err.Error()))
	}
	data := jsonBody(body)
	d := mapOrEmpty(asMapOrEmpty(data["data"]))
	code := -1
	if f, ok := d["error_code"].(float64); ok {
		code = int(f)
	} else if f, ok := data["error_code"].(float64); ok {
		code = int(f)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	desc := asString(d["description"])
	if desc == "" {
		desc = asString(data["message"])
	}
	s.Description = desc
	if code != 0 {
		lower := strings.ToLower(s.Description)
		if strings.Contains(lower, "expired") || strings.Contains(s.Description, "过期") {
			s.Status = "expired"
		}
		return s.state(code), nil
	}
	if st := asString(d["status"]); st != "" {
		s.Status = st
	}
	if ru := asString(d["redirect_url"]); ru != "" {
		s.RedirectURL = ru
	}
	return s.state(0), nil
}

func (s *Session) state(code int) map[string]any {
	confirmed := s.Status == "confirmed"
	return map[string]any{
		"token":       s.token,
		"status":      s.Status,
		"description": s.Description,
		"error_code":  code,
		"expire_time": s.expireTime,
		"confirmed":   confirmed,
	}
}

// CollectCookie confirmed 之后跟随 redirect_url 收票，返回可写盘的 cookie 串。
//
// sessionid 等 10 个鉴权名是在这条回跳链的 Set-Cookie 上下发的，所以必须
// 逐跳收；只取 doubao.REQUIRED_COOKIES 里缺一不可的那份，顺带完成校验。
func (s *Session) CollectCookie() (string, error) {
	s.mu.Lock()
	rawURL := s.RedirectURL
	s.mu.Unlock()
	if rawURL == "" {
		return "", &doubao.DoubaoError{Msg: "未收到 redirect_url，登录尚未确认", Kind: "auth"}
	}
	for i := 0; i < MaxHops; i++ {
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return "", &doubao.DoubaoError{Msg: "回跳 URL 无效: " + err.Error(), Kind: "auth"}
		}
		req.Header = s.headers("")
		// 每一跳都要带上已积累的 cookie（对齐 Python _http_get(url, headers, self.cookies)）：
		// 回跳链要求携带 csrf/ttwid 等前置 cookie 才下发 sessionid
		s.mu.Lock()
		var pairs []string
		for k, v := range s.cookies {
			pairs = append(pairs, k+"="+v)
		}
		s.mu.Unlock()
		if len(pairs) > 0 {
			req.Header.Set("Cookie", strings.Join(pairs, "; "))
		}
		// 诊断日志：URL 的 path + query 参数名、请求 cookie 名、下发 cookie 名——值一律不进日志
		reqNames := make([]string, 0, len(pairs))
		for _, kv := range pairs {
			if i := strings.IndexByte(kv, '='); i > 0 {
				reqNames = append(reqNames, kv[:i])
			}
		}
		u, _ := url.Parse(rawURL)
		qNames := make([]string, 0, len(u.Query()))
		for k := range u.Query() {
			qNames = append(qNames, k)
		}
		s.Log.Info(fmt.Sprintf("回跳跳%d GET path=%s query_keys=%v req_cookies=%v",
			i+1, u.Path, qNames, reqNames))
		resp, err := s.HTTP.Do(req)
		if err != nil {
			return "", &doubao.DoubaoError{Msg: "回跳失败: " + err.Error(), Kind: "auth"}
		}
		s.mergeCookies(resp)
		// 诊断日志：只记 cookie 名与落点，值绝不进日志
		var names []string
		for _, c := range resp.Cookies() {
			names = append(names, c.Name)
		}
		s.Log.Info(fmt.Sprintf("回跳跳%d status=%d location=%q set_cookie=%v",
			i+1, resp.StatusCode, resp.Header.Get("Location"), names))
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		location := resp.Header.Get("Location")
		if resp.StatusCode < 300 || resp.StatusCode > 399 || location == "" {
			break
		}
		rawURL = resolveURL(rawURL, location)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 只校验必需集（2026-10-25 实测 d_ticket / s_v_web_id 回跳链不下发、
	// chat 端点也不要），可选名收到就一并带上。
	var missing []string
	var parts []string
	for _, k := range doubao.REQUIRED_COOKIES {
		if s.cookies[k] == "" {
			missing = append(missing, k)
			continue
		}
		parts = append(parts, k+"="+s.cookies[k])
	}
	for _, k := range doubao.OPTIONAL_COOKIES {
		if v := s.cookies[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	if len(missing) > 0 {
		return "", &doubao.DoubaoError{
			Msg:  "登录回跳后 cookie 仍缺: " + strings.Join(missing, ", "),
			Kind: "auth"}
	}
	return strings.Join(parts, "; "), nil
}

// ── 小工具 ────────────────────────────────────────────────────────────────────

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asMapOrEmpty(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// resolveURL 最小 urljoin（相对 Location 以当前 URL 为基）。
func resolveURL(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
