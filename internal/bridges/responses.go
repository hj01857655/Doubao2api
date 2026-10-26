// package bridges 的 Responses 侧：OpenAI Responses ⇄ Chat Completions 协议桥。
//
// grok-build / Codex 这类 agent 客户端原生只说 Responses 协议（SSE 每帧必须带
// sequence_number，类型级强制）。字段形状以 grok-build 1.0.52 实际使用的
// async-openai fork（rev 95b52eb）的 serde 必填字段为准核对过：
//   - 每个事件必带 sequence_number（本模块从 0 单调递增）
//   - Response 必填：id / object / created_at / model / status / output
//   - ResponseUsage 必填：input_tokens / input_tokens_details.cached_tokens /
//     output_tokens / output_tokens_details.reasoning_tokens / total_tokens
//   - ErrorObject 必填：code + message；IncompleteDetails 必填：reason
package bridges

import (
	"iter"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// 一、请求侧：Responses 请求体 -> Chat Completions 请求体
// ════════════════════════════════════════════════════════════════════════════

// respContentPartToChat Responses 内容块 -> Chat 内容块（不支持的返回 nil 丢弃）。
func respContentPartToChat(part map[string]any) map[string]any {
	if part == nil {
		return nil
	}
	switch asString(part["type"]) {
	case "input_text", "output_text", "text":
		return map[string]any{"type": "text", "text": asString(part["text"])}
	case "input_image":
		var url string
		if u := asMap(part["image_url"]); u != nil {
			url = asString(u["url"])
		} else {
			url = asString(part["image_url"])
		}
		if url != "" {
			return map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}
		}
	}
	// input_file / input_image(file_id) 等 Chat 表达不了的块：丢弃
	return nil
}

// respToChatContent Responses message.content（字符串或块数组）-> Chat content。
func respToChatContent(content any) any {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	blocks, ok := content.([]any)
	if !ok {
		return ""
	}
	parts := []any{}
	for _, p := range blocks {
		if s, ok := p.(string); ok {
			parts = append(parts, map[string]any{"type": "text", "text": s})
			continue
		}
		if c := respContentPartToChat(asMap(p)); c != nil {
			parts = append(parts, c)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		if t, ok := parts[0].(map[string]any); ok && asString(t["type"]) == "text" {
			return asString(t["text"])
		}
	}
	return parts
}

// respInputToMessages instructions + input -> Chat messages。
//
// instructions -> 首条 system；input 字符串 -> 单条 user；连续 function_call
// 合并为一条 assistant.tool_calls 消息；function_call_output -> role=tool；
// reasoning / item_reference 等无 Chat 对应的项跳过。
func respInputToMessages(body map[string]any) []any {
	messages := []any{}
	if instructions, ok := body["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}

	var pendingCalls []map[string]any
	flushCalls := func() {
		if len(pendingCalls) == 0 {
			return
		}
		tcs := make([]any, 0, len(pendingCalls))
		for _, c := range pendingCalls {
			callID := asString(c["call_id"])
			if callID == "" {
				callID = "call_" + hexID(24)
			}
			tcs = append(tcs, map[string]any{
				"id":   callID,
				"type": "function",
				"function": map[string]any{
					"name":      asString(c["name"]),
					"arguments": asString(c["arguments"]),
				},
			})
		}
		messages = append(messages, map[string]any{
			"role": "assistant", "content": nil, "tool_calls": tcs,
		})
		pendingCalls = nil
	}

	switch inp := body["input"].(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", "content": inp})
		return messages
	case []any:
		for _, ii := range inp {
			if s, ok := ii.(string); ok {
				flushCalls()
				messages = append(messages, map[string]any{"role": "user", "content": s})
				continue
			}
			item := asMap(ii)
			if item == nil {
				continue
			}
			t := asString(item["type"])
			_, hasRole := item["role"]
			_, hasContent := item["content"]
			// EasyInput 简写：{"role","content"} 不带 type（OpenAI SDK 直发此形状）
			if t == "" && (hasRole || hasContent) {
				t = "message"
			}
			switch t {
			case "message":
				flushCalls()
				role := asString(item["role"])
				if role == "" {
					role = "user"
				}
				if role == "developer" {
					role = "system"
				}
				if role != "system" && role != "user" && role != "assistant" {
					role = "user"
				}
				messages = append(messages, map[string]any{
					"role": role, "content": respToChatContent(item["content"]),
				})
			case "function_call":
				pendingCalls = append(pendingCalls, item)
			case "function_call_output":
				flushCalls()
				out, ok := item["output"].(string)
				if !ok {
					if item["output"] == nil {
						out = ""
					} else {
						out = string(marshalCompact(item["output"]))
					}
				}
				messages = append(messages, map[string]any{
					"role": "tool", "tool_call_id": asString(item["call_id"]), "content": out,
				})
			}
			// 其它类型（reasoning / item_reference / ...）：无 Chat 对应，跳过
		}
		flushCalls()
	}
	return messages
}

// respToolsToChat Responses tools -> Chat tools；内置工具（web_search 等）丢弃。
func respToolsToChat(tools any) []any {
	list, ok := tools.([]any)
	if !ok {
		return nil
	}
	out := []any{}
	for _, ti := range list {
		t := asMap(ti)
		if t == nil || asString(t["type"]) != "function" {
			continue
		}
		params := t["parameters"]
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        asString(t["name"]),
				"description": asString(t["description"]),
				"parameters":  params,
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// respToolChoiceToChat Responses tool_choice -> Chat tool_choice。
func respToolChoiceToChat(tc any) any {
	switch v := tc.(type) {
	case string:
		if v == "auto" || v == "required" || v == "none" {
			return v
		}
		return "auto"
	case map[string]any:
		if asString(v["type"]) == "function" {
			return map[string]any{
				"type":     "function",
				"function": map[string]any{"name": asString(v["name"])},
			}
		}
	}
	return nil
}

// ResponsesToChat Responses API 请求体 -> Chat Completions 请求体。
// reasoning / text / store / previous_response_id 等无对应项直接丢弃（v1）。
func ResponsesToChat(body map[string]any) map[string]any {
	stream := false
	if b, ok := body["stream"].(bool); ok {
		stream = b
	}
	chat := map[string]any{
		"model":    asString(body["model"]),
		"messages": respInputToMessages(body),
		"stream":   stream,
	}
	if tools := respToolsToChat(body["tools"]); tools != nil {
		chat["tools"] = tools
	}
	if tc := respToolChoiceToChat(body["tool_choice"]); tc != nil {
		chat["tool_choice"] = tc
	}
	for src, dst := range map[string]string{
		"temperature": "temperature", "top_p": "top_p",
		"max_output_tokens": "max_tokens", "parallel_tool_calls": "parallel_tool_calls",
		"stream_options": "stream_options",
	} {
		if v, ok := body[src]; ok && v != nil {
			chat[dst] = v
		}
	}
	return chat
}

// ════════════════════════════════════════════════════════════════════════════
// 二、响应侧公共：输出 item 构造 + usage 映射
// ════════════════════════════════════════════════════════════════════════════

func reasoningItem(itemID, status string) map[string]any {
	// summary 是必填字段；流式思考内容已通过 reasoning_summary_text.delta/done
	// 帧交付给客户端，这里只保骨架
	return map[string]any{
		"type": "reasoning", "id": itemID, "summary": []any{}, "status": status,
	}
}

func messageItem(itemID, text, status string) map[string]any {
	return map[string]any{
		"type": "message", "id": itemID, "role": "assistant", "status": status,
		"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}},
	}
}

func functionItem(itemID, callID, name, arguments, status string) map[string]any {
	return map[string]any{
		"type": "function_call", "id": itemID, "call_id": callID,
		"name": name, "arguments": arguments, "status": status,
	}
}

// usageFromChat Chat usage -> Responses usage（五个字段全部必填，一个都不能省）。
func usageFromChat(u map[string]any) map[string]any {
	if u == nil {
		return nil
	}
	pt := int(asFloat(u["prompt_tokens"]))
	ct := int(asFloat(u["completion_tokens"]))
	rt := 0
	if det := asMap(u["completion_tokens_details"]); det != nil {
		rt = int(asFloat(det["reasoning_tokens"]))
	}
	cached := 0
	if pdet := asMap(u["prompt_tokens_details"]); pdet != nil {
		cached = int(asFloat(pdet["cached_tokens"]))
	}
	total := int(asFloat(u["total_tokens"]))
	if u["total_tokens"] == nil {
		total = pt + ct
	}
	return map[string]any{
		"input_tokens":          pt,
		"input_tokens_details":  map[string]any{"cached_tokens": cached},
		"output_tokens":         ct,
		"output_tokens_details": map[string]any{"reasoning_tokens": rt},
		"total_tokens":          total,
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 三、流式：Chat SSE 字节流 -> Responses SSE 字节流
// ════════════════════════════════════════════════════════════════════════════

type respToolState struct {
	itemID      string
	callID      string
	name        string
	args        string
	outputIndex int
}

type respItemRef struct {
	itemID      string
	outputIndex int
	buf         string
}

type respStreamState struct {
	model         string
	respID        string
	createdAt     int64
	seq           int
	nextIdx       int
	usage         map[string]any
	finishReason  string
	upstreamError *upstreamErr
	interrupted   error
	items         map[int]map[string]any
	order         []int
	reasoning     *respItemRef
	text          *respItemRef
	tools         map[int]*respToolState // chat tool_calls index
}

func newRespStreamState(model string) *respStreamState {
	return &respStreamState{
		model:     model,
		respID:    "resp_" + hexID(32),
		createdAt: time.Now().Unix(),
		items:     map[int]map[string]any{},
		tools:     map[int]*respToolState{},
	}
}

func (st *respStreamState) register(item map[string]any) int {
	idx := st.nextIdx
	st.nextIdx++
	st.items[idx] = item
	st.order = append(st.order, idx)
	return idx
}

// responseObj 组装 Response 对象（六个必填字段一个不少）。
func (st *respStreamState) responseObj(status string, usage map[string]any, errObj map[string]any, incompleteDetails map[string]any) map[string]any {
	output := make([]any, 0, len(st.order))
	for _, i := range st.order {
		output = append(output, st.items[i])
	}
	obj := map[string]any{
		"id": st.respID, "object": "response", "created_at": st.createdAt,
		"model": st.model, "status": status, "output": output,
	}
	if usage != nil {
		obj["usage"] = usage
	}
	if errObj != nil {
		obj["error"] = errObj
	}
	if incompleteDetails != nil {
		obj["incomplete_details"] = incompleteDetails
	}
	if status == "completed" {
		obj["completed_at"] = time.Now().Unix()
	}
	return obj
}

// ChatSSEToResponses Chat SSE 行流 -> Responses SSE 帧流。每帧必带
// sequence_number；收尾保证以 response.completed / response.incomplete /
// response.failed 之一结束；不发 [DONE]（那是 Chat 惯例，Responses 没有）。
// interrupted 由 lines 的读错误判定（上游断流不让异常掀掉 SSE，收尾帧兜底）。
func ChatSSEToResponses(lines SSELines, model string) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		st := newRespStreamState(model)

		frame := func(event map[string]any) bool {
			event["sequence_number"] = st.seq
			st.seq++
			return yield(append([]byte("data: "), append(marshalCompact(event), '\n', '\n')...))
		}

		ensureReasoning := func() (string, int, []byte, bool) {
			if st.reasoning == nil {
				itemID := "rs_" + hexID(24)
				idx := st.register(reasoningItem(itemID, "in_progress"))
				st.reasoning = &respItemRef{itemID: itemID, outputIndex: idx}
				ev := map[string]any{
					"type": "response.output_item.added", "output_index": idx,
					"item": st.items[idx],
				}
				ev["sequence_number"] = st.seq
				st.seq++
				return itemID, idx, append([]byte("data: "), append(marshalCompact(ev), '\n', '\n')...), true
			}
			return st.reasoning.itemID, st.reasoning.outputIndex, nil, false
		}
		ensureText := func() (string, int, []byte, bool) {
			if st.text == nil {
				itemID := "msg_" + hexID(24)
				idx := st.register(messageItem(itemID, "", "in_progress"))
				st.text = &respItemRef{itemID: itemID, outputIndex: idx}
				ev := map[string]any{
					"type": "response.output_item.added", "output_index": idx,
					"item": st.items[idx],
				}
				ev["sequence_number"] = st.seq
				st.seq++
				return itemID, idx, append([]byte("data: "), append(marshalCompact(ev), '\n', '\n')...), true
			}
			return st.text.itemID, st.text.outputIndex, nil, false
		}

		yield(append([]byte("data: "), append(marshalCompact(map[string]any{
			"type":            "response.created",
			"response":        st.responseObj("in_progress", nil, nil, nil),
			"sequence_number": st.seq,
		}), '\n', '\n')...))
		st.seq++
		yield(append([]byte("data: "), append(marshalCompact(map[string]any{
			"type":            "response.in_progress",
			"response":        st.responseObj("in_progress", nil, nil, nil),
			"sequence_number": st.seq,
		}), '\n', '\n')...))
		st.seq++

		for chunk := range chatChunks(lines.Seq) {
			if chunk.upstreamError != nil {
				st.upstreamError = chunk.upstreamError
				break
			}
			if chunk.usage != nil {
				st.usage = usageFromChat(chunk.usage)
			}
			st.finishReason = pickFinish(chunk, st.finishReason)

			// 1) reasoning_content -> 思考摘要文本 delta
			if rc := chunk.reasoningContent; rc != "" {
				itemID, idx, added, _ := ensureReasoning()
				if added != nil && !yield(added) {
					return
				}
				st.reasoning.buf += rc
				if !frame(map[string]any{
					"type":    "response.reasoning_summary_text.delta",
					"item_id": itemID, "output_index": idx, "summary_index": 0, "delta": rc,
				}) {
					return
				}
			}

			// 2) content -> 正文文本 delta
			if ct := chunk.content; ct != "" {
				itemID, idx, added, _ := ensureText()
				if added != nil && !yield(added) {
					return
				}
				st.text.buf += ct
				if !frame(map[string]any{
					"type":    "response.output_text.delta",
					"item_id": itemID, "output_index": idx, "content_index": 0, "delta": ct,
				}) {
					return
				}
			}

			// 3) tool_calls -> function_call item + 参数 delta
			for _, tc := range chunk.toolCalls {
				stl, ok := st.tools[tc.index]
				if !ok {
					callID := tc.id
					if callID == "" {
						callID = "call_" + hexID(24)
					}
					itemID := "fc_" + hexID(24)
					idx := st.register(functionItem(itemID, callID, tc.name, "", "in_progress"))
					stl = &respToolState{
						itemID: itemID, callID: callID, name: tc.name,
						outputIndex: idx,
					}
					st.tools[tc.index] = stl
					if !frame(map[string]any{
						"type": "response.output_item.added", "output_index": idx,
						"item": st.items[idx],
					}) {
						return
					}
				}
				if tc.name != "" {
					stl.name = tc.name
					st.items[stl.outputIndex]["name"] = tc.name
				}
				if tc.arguments != "" {
					stl.args += tc.arguments
					st.items[stl.outputIndex]["arguments"] = stl.args
					if !frame(map[string]any{
						"type":    "response.function_call_arguments.delta",
						"item_id": stl.itemID, "output_index": stl.outputIndex,
						"delta": tc.arguments,
					}) {
						return
					}
				}
			}
		}
		// 行流结束（正常 EOF 或读错误）；读错误 = 上游断流
		if lines.Err != nil {
			st.interrupted = lines.Err()
		}

		// 逐项收尾：先发各类型 done 帧（按 output 序），再发 output_item.done
		for _, idx := range st.order {
			item := st.items[idx]
			switch asString(item["type"]) {
			case "reasoning":
				if st.reasoning != nil && st.reasoning.outputIndex == idx && st.reasoning.buf != "" {
					if !frame(map[string]any{
						"type":    "response.reasoning_summary_text.done",
						"item_id": item["id"], "output_index": idx,
						"summary_index": 0, "text": st.reasoning.buf,
					}) {
						return
					}
				}
				item["status"] = "completed"
			case "message":
				if st.text != nil && st.text.outputIndex == idx {
					if !frame(map[string]any{
						"type":    "response.output_text.done",
						"item_id": item["id"], "output_index": idx,
						"content_index": 0, "text": st.text.buf,
					}) {
						return
					}
					// 终帧必须携带全文：把攒的 buffer 写回 item，
					// 否则 output_item.done / response.completed 里正文为空
					content := asList(item["content"])
					if len(content) > 0 {
						if c0 := asMap(content[0]); c0 != nil {
							c0["text"] = st.text.buf
						}
					}
				}
				item["status"] = "completed"
			case "function_call":
				var name, args string
				for _, s := range st.tools {
					if s.outputIndex == idx {
						name, args = s.name, s.args
						break
					}
				}
				if name == "" {
					name = asString(item["name"])
				}
				if !frame(map[string]any{
					"type": "response.function_call_arguments.done",
					"name": name, "item_id": item["id"],
					"output_index": idx, "arguments": args,
				}) {
					return
				}
				item["status"] = "completed"
			}
			if !frame(map[string]any{
				"type": "response.output_item.done", "output_index": idx, "item": item,
			}) {
				return
			}
		}

		// 终局事件：failed / incomplete / completed 三选一
		switch {
		case st.upstreamError != nil:
			frame(map[string]any{
				"type": "response.failed",
				"response": st.responseObj("failed", st.usage, map[string]any{
					"code": st.upstreamError.code, "message": st.upstreamError.msg,
				}, nil),
			})
		case st.interrupted != nil:
			if len(st.order) > 0 {
				frame(map[string]any{
					"type": "response.incomplete",
					"response": st.responseObj("incomplete", st.usage, nil,
						map[string]any{"reason": "stream_interrupted"}),
				})
			} else {
				frame(map[string]any{
					"type": "response.failed",
					"response": st.responseObj("failed", st.usage, map[string]any{
						"code": "stream_interrupted", "message": st.interrupted.Error(),
					}, nil),
				})
			}
		case st.finishReason == "length":
			frame(map[string]any{
				"type": "response.incomplete",
				"response": st.responseObj("incomplete", st.usage, nil,
					map[string]any{"reason": "max_output_tokens"}),
			})
		default:
			frame(map[string]any{
				"type":     "response.completed",
				"response": st.responseObj("completed", st.usage, nil, nil),
			})
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 四、非流式：Chat JSON -> Responses JSON
// ════════════════════════════════════════════════════════════════════════════

// ChatJSONToResponses 非流式 Chat Completions JSON -> Responses Response 对象。
func ChatJSONToResponses(chat map[string]any, model string) map[string]any {
	if model == "" {
		model = asString(chat["model"])
	}
	st := newRespStreamState(model)
	msg := map[string]any{}
	if choices := asList(chat["choices"]); len(choices) > 0 {
		if c := asMap(choices[0]); c != nil {
			if m := asMap(c["message"]); m != nil {
				msg = m
			}
			if fr := asString(c["finish_reason"]); fr != "" {
				st.finishReason = fr
			}
		}
	}
	if u := asMap(chat["usage"]); u != nil {
		st.usage = usageFromChat(u)
	}

	// output 顺序：reasoning -> message -> function_call（与流式一致）
	if rc, ok := msg["reasoning_content"].(string); ok && rc != "" {
		st.register(reasoningItem("rs_"+hexID(24), "completed"))
	}
	if text := asString(msg["content"]); text != "" {
		st.register(messageItem("msg_"+hexID(24), text, "completed"))
	}
	for _, tci := range asList(msg["tool_calls"]) {
		tc := asMap(tci)
		if tc == nil {
			continue
		}
		fn := asMap(tc["function"])
		if fn == nil {
			fn = map[string]any{}
		}
		callID := asString(tc["id"])
		if callID == "" {
			callID = "call_" + hexID(24)
		}
		st.register(functionItem("fc_"+hexID(24), callID,
			asString(fn["name"]), asString(fn["arguments"]), "completed"))
	}

	if st.finishReason == "length" {
		return st.responseObj("incomplete", st.usage, nil,
			map[string]any{"reason": "max_output_tokens"})
	}
	return st.responseObj("completed", st.usage, nil, nil)
}

// ── 五、错误对象与「客户端要流、上游只给非流结果」的最小 SSE 序列 ─────────────

// ErrorResponseObj 构造最小可用的 failed Response 对象（六个必填字段齐全）
// （对应 error_response_obj）。
func ErrorResponseObj(model, code, message string) map[string]any {
	if code == "" {
		code = "upstream_error"
	}
	if message == "" {
		message = "upstream error"
	}
	return map[string]any{
		"id":         "resp_" + hexID(32),
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      model,
		"status":     "failed",
		"output":     []any{},
		"error":      map[string]any{"code": code, "message": message},
	}
}

// ResponseJSONToSSE 完整 Responses JSON -> 最小 SSE 帧序列。用于客户端要流式、
// 但上游只给了非流式结果（或错误）的场景：created -> in_progress -> 逐 item
// added -> 终局事件（按 status 选）。每帧带 sequence_number，与流式路径的帧
// 形状完全一致（对应 response_json_to_sse）。
func ResponseJSONToSSE(responseObj map[string]any) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		seq := 0
		frame := func(event map[string]any) bool {
			event["sequence_number"] = seq
			seq++
			return yield(append([]byte("data: "), append(marshalCompact(event), '\n', '\n')...))
		}
		base := map[string]any{}
		for k, v := range responseObj {
			base[k] = v
		}
		base["status"] = "in_progress"
		base["output"] = []any{}
		delete(base, "completed_at")
		delete(base, "error")
		delete(base, "incomplete_details")
		if !frame(map[string]any{"type": "response.created", "response": base}) {
			return
		}
		if !frame(map[string]any{"type": "response.in_progress", "response": base}) {
			return
		}
		output, _ := responseObj["output"].([]any)
		for idx, item := range output {
			if !frame(map[string]any{
				"type":         "response.output_item.added",
				"output_index": idx, "item": item,
			}) {
				return
			}
		}
		status := asString(responseObj["status"])
		if status == "" {
			status = "completed"
		}
		terminal := map[string]string{
			"completed": "response.completed", "failed": "response.failed",
			"incomplete": "response.incomplete",
		}[status]
		if terminal == "" {
			terminal = "response.completed"
		}
		frame(map[string]any{"type": terminal, "response": responseObj})
	}
}
