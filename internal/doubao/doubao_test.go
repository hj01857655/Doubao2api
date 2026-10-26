package doubao

import (
	"encoding/json"
	"strings"
	"testing"
)

// fakeCookie 10 个必需名的短值假 cookie（测试凭据 <24 字符，避免输出管道脱敏）。
const fakeCookie = "sessionid=fake; sessionid_ss=fake; sid_tt=fake; sid_guard=fake; " +
	"uid_tt=fake; uid_tt_ss=fake; d_ticket=fake; ttwid=fake; s_v_web_id=fake; " +
	"passport_csrf_token=fake"

// fixture 流：docs/03 那次真实响应的逐帧复刻（tests/fixture_capture.txt 同源）。
var fixtureLines = []string{
	`id: 0`,
	`event: SSE_HEARTBEAT`,
	`data: {}`,
	``,
	`event: SSE_ACK`,
	`data: {"query_list":[{"question_id":"58756254450320130"}],"ack_client_meta":{"conversation_id":"38447554619021570","section_id":"38447554619021826"},"timeout_conf":{"retry_interval_ms":1000}}`,
	``,
	`event: STREAM_MSG_NOTIFY`,
	`data: {"content":{"content_block":[{"block_type":10000,"content":{"text_block":{"text":"你"}}}],"ext":{"model_type":"38"}},"meta":{"message_id":"58756254450326274"}}`,
	``,
	`event: STREAM_CHUNK`,
	`data: {"patch_op":[{"patch_object":1,"patch_value":{"content_block":[{"content":{"text_block":{"text":"好"}}}]}}]}`,
	``,
	`event: STREAM_CHUNK`,
	`data: {"patch_op":[{"patch_object":111,"patch_value":{"tts_content":"好"}}]}`,
	``,
	`event: CHUNK_DELTA`,
	`data: {"text":"呀！有什么我可以"}`,
	``,
	`event: CHUNK_DELTA`,
	`data: {"text":"帮你的吗？"}`,
	``,
	`event: SSE_REPLY_END`,
	`data: {"end_type":1,"msg_finish_attr":{"brief":"你好呀！有什么我可以帮你的吗？"}}`,
	``,
	`event: STREAM_CHUNK`,
	`data: {"patch_op":[{"patch_object":50,"patch_value":{"ext":{"sp_v2":"[{\"suggest_type\":1,\"content\":\" 你能做些什么？\"},{\"suggest_type\":1,\"content\":\" 你是谁？\"},{\"suggest_type\":1,\"content\":\" 你是怎么学习的？\"}]"}}}]}`,
	``,
	`event: SSE_REPLY_END`,
	`data: {"end_type":3}`,
	``,
}

// feed 把 SSE 文本喂进解析器 + 状态机，收集规整化事件。
func feed(t *testing.T, lines []string) ([]Event, *StreamState) {
	t.Helper()
	st := newStreamState()
	endStages := map[string]bool{}
	cur := map[string]string{}
	var out []Event
	for _, line := range lines {
		ev, data, ok := ParseSSELine(line, cur)
		if !ok {
			continue
		}
		outEvs, err := handleEvent(ev, data, st, endStages)
		if err != nil {
			t.Fatalf("handleEvent(%s) 意外出错: %v", ev, err)
		}
		out = append(out, outEvs...)
		if endStages["final"] {
			break
		}
	}
	return out, st
}

func TestParseFixture(t *testing.T) {
	out, st := feed(t, fixtureLines)
	var deltas []string
	var bodyEnd, done map[string]any
	for _, ev := range out {
		switch ev.Kind {
		case "delta":
			deltas = append(deltas, ev.Text)
		case "body_end":
			bodyEnd = ev.Done
		case "done":
			done = ev.Done
		}
	}
	// 两条正文通道不重叠，tts 镜像(111)不混入正文
	want := []string{"你", "好", "呀！有什么我可以", "帮你的吗？"}
	if strings.Join(deltas, "|") != strings.Join(want, "|") {
		t.Errorf("delta 序列 = %v, want %v", deltas, want)
	}
	if got, _ := done["text"].(string); got != "你好呀！有什么我可以帮你的吗？" {
		t.Errorf("拼接正文 = %q", got)
	}
	if bodyEnd == nil {
		t.Fatal("end_type:1 应提前发 body_end")
	}
	if bodyEnd["conversation_id"] != "38447554619021570" {
		t.Errorf("body_end conversation_id = %v", bodyEnd["conversation_id"])
	}
	if done["answer_message_id"] != "58756254450326274" {
		t.Errorf("answer_message_id 来自 STREAM_MSG_NOTIFY, got %v", done["answer_message_id"])
	}
	if done["model_type"] != "38" {
		t.Errorf("服务端真实 model_type = %v", done["model_type"])
	}
	if st.QuestionID != "58756254450320130" {
		t.Errorf("question_id 来自 SSE_ACK, got %q", st.QuestionID)
	}
	wantSugg := []string{"你能做些什么？", "你是谁？", "你是怎么学习的？"}
	sugg := done["suggestions"].([]string)
	if strings.Join(sugg, "|") != strings.Join(wantSugg, "|") {
		t.Errorf("建议取最后一帧全量 = %v", sugg)
	}
	channels := done["channels"].([]string)
	if strings.Join(channels, ",") != "chunk_delta,stream_chunk_patch1,stream_msg_notify" {
		t.Errorf("用到的正文通道 = %v", channels)
	}
}

func TestBriefNeverOverridesText(t *testing.T) {
	// 真实观测：正文 '1\n2\n…12' 时 brief 变 '123456789101112'（换行被吃）。
	// brief 是会话列表预览串，绝不覆盖拼接出的正文。
	lines := []string{
		`event: STREAM_MSG_NOTIFY`,
		`data: {"content":{"content_block":[{"content":{"text_block":{"text":"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"}}}]},"meta":{"message_id":"m1"}}`,
		``,
		`event: SSE_REPLY_END`,
		`data: {"end_type":1,"msg_finish_attr":{"brief":"123456789101112"}}`,
		``,
		`event: SSE_REPLY_END`,
		`data: {"end_type":3}`,
		``,
	}
	out, _ := feed(t, lines)
	for _, ev := range out {
		if ev.Kind == "done" {
			want := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"
			if ev.Done["text"] != want {
				t.Errorf("换行型正文被 brief 篡改: %q", ev.Done["text"])
			}
			return
		}
	}
	t.Fatal("没有 done 事件")
}

func TestStreamErrorBranches(t *testing.T) {
	cases := []struct {
		name     string
		lines    []string
		wantKind string
		wantHTTP int
		retry    bool
	}{
		{"风控滑块", []string{
			`event: STREAM_ERROR`,
			`data: {"error_code":710022004,"error_msg":"rate limited","extra":{"decision":"{\"type\":\"verify\",\"subtype\":\"slide\"}"}}`,
			``}, "risk", 503, false},
		{"过频", []string{
			`event: STREAM_ERROR`,
			`data: {"error_code":710022002,"error_msg":"当前服务访问频繁，请稍后重试"}`,
			``}, "rate", 429, true},
		{"网关身份错误", []string{
			`event: gateway-error`,
			`data: {"code":"Internal","message":"ErrorX:code=710012000 message=[user invalid]"}`,
			``}, "auth", 401, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newStreamState()
			st.LogID = "L1"
			cur := map[string]string{}
			var got *DoubaoError
			for _, line := range tc.lines {
				ev, data, ok := ParseSSELine(line, cur)
				if !ok {
					continue
				}
				_, err := handleEvent(ev, data, st, map[string]bool{})
				if err != nil {
					got = err
					break
				}
			}
			if got == nil {
				t.Fatal("应产出错误")
			}
			if got.Kind != tc.wantKind {
				t.Errorf("kind = %s, want %s", got.Kind, tc.wantKind)
			}
			if got.ToHTTP() != tc.wantHTTP {
				t.Errorf("to_http = %d, want %d", got.ToHTTP(), tc.wantHTTP)
			}
			if got.Retryable() != tc.retry {
				t.Errorf("retryable = %v, want %v", got.Retryable(), tc.retry)
			}
			if tc.name == "风控滑块" {
				if !strings.Contains(got.Msg, "verify") || !strings.Contains(got.Msg, "slide") {
					t.Errorf("错误信息应含 verify/slide: %s", got.Msg)
				}
				if !strings.Contains(got.Msg, "logid=L1") {
					t.Errorf("错误信息应带 logid: %s", got.Msg)
				}
			}
		})
	}
}

func TestResolveModelAndAliases(t *testing.T) {
	canon, mode, err := ResolveModel("doubao-pro")
	if err != nil || canon != "doubao-pro" || mode.ModelItemKey != "5" {
		t.Errorf("doubao-pro 解析: %v %v %v", canon, mode, err)
	}
	if _, m, _ := ResolveModel("doubao"); m.ModelItemKey != MODES[DEFAULT_MODEL].ModelItemKey {
		t.Errorf("别名 doubao 应落默认档")
	}
	if _, m, _ := ResolveModel("seed-lite-7b"); m.Display != "豆包 2.1 Lite" {
		t.Errorf("直接传 model_item_key 应命中: %v", m)
	}
	if _, _, err := ResolveModel("no-such"); err == nil {
		t.Error("未知模型应报错")
	}
}

func TestBuildQueryConstraints(t *testing.T) {
	q := BuildQuery()
	for _, must := range []string{"a_bogus", "msToken"} {
		if strings.Contains(q, must) {
			t.Errorf("query 不应含 %s（实测不校验，docs/05）", must)
		}
	}
	for _, must := range []string{"fp=", "tea_uuid=", "device_id=", "web_tab_id=",
		"chromium_version=", "pkg_type=", "tz_name=", "client_platform="} {
		if !strings.Contains(q, must) {
			t.Errorf("设备参数缺失 %s", must)
		}
	}
	if !strings.Contains(q, "fp=verify_"+DEVICE_ID) {
		t.Error("fp 必须 == verify_<device_id>")
	}
	if !strings.Contains(q, "tea_uuid="+DEVICE_ID) {
		t.Error("tea_uuid 必须 == device_id")
	}
}

func TestBuildEnvelopeConstraints(t *testing.T) {
	_, mode, _ := ResolveModel("doubao-pro")
	env := BuildEnvelope("测试", mode, "", "", "")
	if env["option"].(map[string]any)["need_create_conversation"] != true {
		t.Error("首轮 need_create_conversation=True")
	}
	if env["client_meta"].(map[string]any)["conversation_id"] != "" {
		t.Error("首轮 conversation_id 留空")
	}
	env2 := BuildEnvelope("测试", mode, "38447554619021570", "", "")
	if env2["option"].(map[string]any)["need_create_conversation"] != false {
		t.Error("续轮 need_create_conversation=False")
	}
	if env2["client_meta"].(map[string]any)["conversation_id"] != "38447554619021570" {
		t.Error("续轮带上 conversation_id")
	}
	ext := env["ext"].(map[string]any)
	initExt := env["option"].(map[string]any)["conversation_init_ext"].(map[string]any)
	if initExt["model_item_key"] != "5" {
		t.Errorf("conversation_init_ext.model_item_key = %v", initExt["model_item_key"])
	}
	for k, v := range ext {
		if _, ok := v.(string); !ok {
			t.Errorf("ext.%s 值不是字符串: %T", k, v)
		}
	}
	blk := env["messages"].([]any)[0].(map[string]any)["content_block"].([]any)[0].(map[string]any)
	if n, ok := blk["block_type"].(int); !ok || n != 10000 {
		t.Errorf("块类型 10000=文本, got %T %v", blk["block_type"], blk["block_type"])
	}
}

func TestBuildCookie(t *testing.T) {
	got, err := BuildCookie(fakeCookie)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "=") != 10 {
		t.Errorf("cookie 应裁到 10 名: %q", got)
	}
	if strings.Contains(strings.ToUpper(got), "LARK") {
		t.Error("cookie 不应含 LARK 令牌")
	}
	if _, err := BuildCookie("ttwid=1"); err == nil {
		t.Error("缺必需 cookie 应报错")
	} else if de, ok := err.(*DoubaoError); !ok || de.Kind != "auth" {
		t.Errorf("缺失报错 kind=auth, got %v", err)
	}
	// 2026-10-25 扫码实测：缺 d_ticket/s_v_web_id 的 8 名直接打真实端点 PASS，
	// 可选名缺席不应报错
	got8, err := BuildCookie("sessionid=fake; sessionid_ss=fake; sid_tt=fake; sid_guard=fake; " +
		"uid_tt=fake; uid_tt_ss=fake; ttwid=fake; passport_csrf_token=fake")
	if err != nil {
		t.Fatalf("8 名（无可选名）应可用: %v", err)
	}
	if strings.Count(got8, "=") != 8 {
		t.Errorf("8 名 cookie 不应被改名: %q", got8)
	}
}

func TestParseSSELineMultiData(t *testing.T) {
	// 多 data: 行拼接（无分隔符，对齐 Python 实现）；无 event 行的帧直接丢弃
	cur := map[string]string{}
	if _, _, ok := ParseSSELine("data: {\"a\":", cur); ok {
		t.Fatal("帧未完整")
	}
	if _, _, ok := ParseSSELine("data: 1}", cur); ok {
		t.Fatal("帧未完整")
	}
	if _, _, ok := ParseSSELine("", cur); ok {
		t.Fatal("无 event 行的帧应被丢弃")
	}
	cur2 := map[string]string{}
	ParseSSELine("event: CHUNK_DELTA", cur2)
	ParseSSELine("data: {\"a\":", cur2)
	ParseSSELine("data: 1}", cur2)
	ev, data, ok := ParseSSELine("", cur2)
	if !ok || ev != "CHUNK_DELTA" {
		t.Fatalf("空行出帧: %v %v", ev, ok)
	}
	if data["a"] != float64(1) {
		b, _ := json.Marshal(data)
		t.Errorf("data 拼接结果 = %s", b)
	}
}
