// package bridges 内部共享：Chat SSE 行流 -> 结构化 delta 流的解析器，
// Anthropic / Responses 两侧同一套循环（错误帧检测、usage 记录、choices[0] 展开）。
package bridges

import (
	"encoding/json"
	"iter"
	"strings"
)

type upstreamErr struct{ code, msg string }

type toolCallDelta struct {
	index     int
	id        string
	name      string
	arguments string
}

type chatDelta struct {
	content          string
	reasoningContent string // Responses 侧独有（delta.reasoning_content）
	toolCalls        []toolCallDelta
	finishReason     string
	usage            map[string]any
	upstreamError    *upstreamErr
}

// chatChunks 把 Chat SSE 行流解析成结构化 delta。上游错误帧（{"error":
// {"message":...}} 或 {"error_code","error_msg"}）产出一个带 upstreamError 的
// 元素后终止迭代，与 Python 侧 break 语义一致。
func chatChunks(lines iter.Seq[string]) iter.Seq[*chatDelta] {
	return func(yield func(*chatDelta) bool) {
		seenTools := map[int]bool{}
		for line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(line[len("data:"):])
			if payload == "" || payload == "[DONE]" {
				continue
			}
			var chunk map[string]any
			if json.Unmarshal([]byte(payload), &chunk) != nil || chunk == nil {
				continue
			}

			d := &chatDelta{}

			// 项目自定义错误帧：{"error":{"message":...}}（上游错误帧转出来的
			// OpenAI error 帧）或 {"error_code","error_msg"} 原样帧
			if err := asMap(chunk["error"]); err != nil {
				if msg := asString(err["message"]); msg != "" {
					code := asString(err["code"])
					if code == "" {
						code = "upstream_error"
					}
					d.upstreamError = &upstreamErr{code: code, msg: msg}
					yield(d)
					return
				}
			}
			if ec, em := asString(chunk["error_code"]), asString(chunk["error_msg"]); ec != "" || em != "" {
				code := ec
				if code == "" {
					code = "upstream_error"
				}
				msg := em
				if msg == "" {
					msg = "upstream error"
				}
				d.upstreamError = &upstreamErr{code: code, msg: msg}
				yield(d)
				return
			}

			if u := asMap(chunk["usage"]); u != nil {
				d.usage = u
			}

			choices := asList(chunk["choices"])
			if len(choices) == 0 {
				if d.usage != nil {
					yield(d)
				}
				continue
			}
			choice := asMap(choices[0])
			if choice == nil {
				continue
			}
			d.finishReason = asString(choice["finish_reason"])
			delta := asMap(choice["delta"])
			if delta == nil {
				if d.usage != nil || d.finishReason != "" {
					yield(d)
				}
				continue
			}

			d.content = asString(delta["content"])
			d.reasoningContent = asString(delta["reasoning_content"])
			for _, tci := range asList(delta["tool_calls"]) {
				tc := asMap(tci)
				if tc == nil {
					continue
				}
				idx := -1
				if f, ok := tc["index"].(float64); ok {
					idx = int(f)
				}
				if idx < 0 {
					// 无 index 的分片：按已见 tool index 数兜底（与 Python 一致）
					idx = len(seenTools)
				}
				seenTools[idx] = true
				fn := asMap(tc["function"])
				if fn == nil {
					fn = map[string]any{}
				}
				d.toolCalls = append(d.toolCalls, toolCallDelta{
					index:     idx,
					id:        asString(tc["id"]),
					name:      asString(fn["name"]),
					arguments: asString(fn["arguments"]),
				})
			}
			if d.content != "" || d.reasoningContent != "" || len(d.toolCalls) > 0 || d.usage != nil || d.finishReason != "" {
				yield(d)
			}
		}
	}
}

// pickFinish 非空 finish_reason 才覆盖当前值。
func pickFinish(d *chatDelta, cur string) string {
	if d.finishReason != "" {
		return d.finishReason
	}
	return cur
}
