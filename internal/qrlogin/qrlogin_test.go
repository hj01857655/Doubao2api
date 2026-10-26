package qrlogin

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"doubao2api/internal/doubao"
)

// 假 passport：csrf_token -> get_qrcode -> check_qrconnect -> 回跳链逐跳下发
// 真实链的 8 个必需 cookie 名。整条链不跟随跳转（Session 自带 CheckRedirect）。
func TestQrLoginFullFlow(t *testing.T) {
	var polls atomic.Int64
	var passport *httptest.Server
	passport = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/passport/safe/csrf_token/":
			if r.URL.Query().Get("aid") != LoginAID {
				t.Errorf("csrf aid = %s, want %s（登录面 497858，不是 chat 面）",
					r.URL.Query().Get("aid"), LoginAID)
			}
			http.SetCookie(w, &http.Cookie{Name: "passport_csrf_token", Value: "csrf-1"})
			fmt.Fprint(w, `{"data":{"passport_csrf_token":"csrf-1"},"message":"success"}`)
		case "/passport/web/get_qrcode/":
			if r.Header.Get("x-tt-passport-csrf-token") != "csrf-1" {
				t.Error("get_qrcode 必须带头 x-tt-passport-csrf-token（塞 query 不生效）")
			}
			if r.URL.Query().Get("next") == "" {
				t.Error("get_qrcode 必须带 next")
			}
			if r.Header.Get("Referer") != loginReferer {
				t.Errorf("referer = %q", r.Header.Get("Referer"))
			}
			png := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
			fmt.Fprintf(w,
				`{"data":{"error_code":0,"token":"tok123","qrcode":"%s","expire_time":180}}`, png)
		case "/passport/web/check_qrconnect/":
			if r.URL.Query().Get("token") != "tok123" {
				t.Errorf("token = %q", r.URL.Query().Get("token"))
			}
			if n := polls.Add(1); n == 1 {
				fmt.Fprint(w, `{"data":{"error_code":0,"status":"scanned","description":"扫码成功，待确认"}}`)
				return
			}
			fmt.Fprintf(w,
				`{"data":{"error_code":0,"status":"confirmed","redirect_url":"%s/r1"}}`, passport.URL)
		case "/r1":
			// 回跳请求必须带上已积累的 cookie（真实链要求携带 csrf/ttwid 才下发 sessionid）
			if ck := r.Header.Get("Cookie"); !strings.Contains(ck, "passport_csrf_token=csrf-1") {
				t.Errorf("回跳请求未携带前置 cookie: %q", ck)
			}
			http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "v1"})
			http.SetCookie(w, &http.Cookie{Name: "sessionid_ss", Value: "v1"})
			http.SetCookie(w, &http.Cookie{Name: "sid_tt", Value: "v1"})
			http.SetCookie(w, &http.Cookie{Name: "sid_guard", Value: "v1"})
			http.SetCookie(w, &http.Cookie{Name: "uid_tt", Value: "v1"})
			http.Redirect(w, r, "/r2", http.StatusFound)
		case "/r2":
			// 真实回跳链形态（2026-10-25 实测）：d_ticket / s_v_web_id 不下发
			http.SetCookie(w, &http.Cookie{Name: "uid_tt_ss", Value: "v2"})
			http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "v2"})
			http.SetCookie(w, &http.Cookie{Name: "passport_csrf_token", Value: "csrf-2"})
			fmt.Fprint(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer passport.Close()

	s := NewSession()
	s.Base = passport.URL // httptest 注入，整条流程离线
	s.Next = passport.URL

	info, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if info["token"] != "tok123" {
		t.Errorf("token = %v", info["token"])
	}
	// get_qrcode 的 qrcode 是裸 base64 PNG，前端要 data URI
	if uri, _ := info["qrcode_data_uri"].(string); !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Errorf("qrcode_data_uri = %v", uri)
	}
	if _, ok := info["qrcode"]; !ok {
		t.Error("Start 应保留 qrcode 原串（handler 侧才删）")
	}

	// 第一次 poll：scanned
	st, err := s.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if st["status"] != "scanned" || st["confirmed"] != false {
		t.Errorf("第一次 poll state = %v", st)
	}

	// 第二次 poll：confirmed + redirect_url
	st, err = s.Poll()
	if err != nil {
		t.Fatalf("Poll#2: %v", err)
	}
	if st["confirmed"] != true {
		t.Errorf("第二次 poll state = %v", st)
	}

	cookie, err := s.CollectCookie()
	if err != nil {
		t.Fatalf("CollectCookie: %v", err)
	}
	// 回跳链逐跳收票：r1 下 5 个、r2 下 3 个（含覆盖 csrf）——真实链只给
	// 8 个必需名，缺可选名不挡登录
	if n := strings.Count(cookie, "="); n != 8 {
		t.Errorf("cookie 应收齐 8 名, got %d: %s", n, cookie)
	}
	for _, k := range doubao.REQUIRED_COOKIES {
		if !strings.Contains(cookie, k+"=") {
			t.Errorf("cookie 缺 %s: %s", k, cookie)
		}
	}
	for _, k := range doubao.OPTIONAL_COOKIES {
		if strings.Contains(cookie, k+"=") {
			t.Errorf("可选名 %s 未下发就不应出现在收票里: %s", k, cookie)
		}
	}
}

// CollectCookie 在无 redirect_url 时应直接拒绝（登录尚未确认）。
func TestCollectCookieWithoutRedirect(t *testing.T) {
	s := NewSession()
	s.Base = "http://127.0.0.1:9" // 黑洞
	_, err := s.CollectCookie()
	if err == nil {
		t.Fatal("无 redirect_url 应报错")
	}
	if !strings.Contains(err.Error(), "redirect_url") {
		t.Errorf("错误信息 = %v", err)
	}
}

// 缺必需名的回跳链：收票校验要兜底报缺（缺一不可）。
func TestCollectCookieMissingNames(t *testing.T) {
	passport := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "only-one"})
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer passport.Close()

	s := NewSession()
	s.Base = passport.URL
	s.RedirectURL = passport.URL
	_, err := s.CollectCookie()
	if err == nil {
		t.Fatal("缺必需 cookie 应报错")
	}
	if !strings.Contains(err.Error(), "仍缺") {
		t.Errorf("错误信息应列缺失名: %v", err)
	}
}

// Poll 前没 Start：直接拒绝。
func TestPollWithoutStart(t *testing.T) {
	s := NewSession()
	s.Base = "http://127.0.0.1:9"
	_, err := s.Poll()
	if err == nil || !strings.Contains(err.Error(), "Start") {
		t.Errorf("未 Start 应报错, got %v", err)
	}
}
