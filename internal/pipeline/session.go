package pipeline

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
)

// sessionIDPrefix 让生成的会话标识在日志与上游侧都容易辨认。
const sessionIDPrefix = "sess-"

// SessionIDFromBody 从请求体中推导出一个「每段对话稳定」的会话标识。
//
// 设计依据：对话历史是累积的，同一段对话里第一条 user 消息始终不变，
// 因此用它的内容做哈希，可以在整个对话周期内保持稳定，
// 同时不同对话之间天然区分开——这正是上游要求的
// "为每段对话发送稳定会话 ID" 的语义。
//
// 若请求体为空、无法解析或找不到 user 消息，则退化为对整体请求体求哈希，
// 保证任何情况下都会返回一个非空值。
func SessionIDFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	seed := extractSessionSeed(body)
	if len(seed) == 0 {
		seed = body
	}
	sum := sha1.Sum(seed)
	return sessionIDPrefix + hex.EncodeToString(sum[:])[:24]
}

// extractSessionSeed 按已知协议结构定位第一条 user 消息的内容。
// 依次尝试 OpenAI / Anthropic 的 messages[]，以及 Gemini 的 contents[]。
func extractSessionSeed(body []byte) []byte {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}
	if raw, ok := root["messages"]; ok {
		if seed := firstUserContent(raw); len(seed) > 0 {
			return seed
		}
	}
	if raw, ok := root["contents"]; ok {
		if seed := firstUserContent(raw); len(seed) > 0 {
			return seed
		}
	}
	return nil
}

// firstUserContent 在消息数组中寻找第一条 role == "user" 的记录。
// 数组顺序是确定的，因此同一份请求体永远得到同一个结果，可复现。
func firstUserContent(raw json.RawMessage) []byte {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	for _, item := range items {
		var role string
		if roleRaw, ok := item["role"]; ok {
			if err := json.Unmarshal(roleRaw, &role); err != nil {
				continue
			}
		}
		if role != "user" {
			continue
		}
		// OpenAI / Anthropic 使用 content 字段
		if content, ok := item["content"]; ok && isMeaningfulJSON(content) {
			return content
		}
		// Gemini 使用 parts 字段
		if parts, ok := item["parts"]; ok && isMeaningfulJSON(parts) {
			return parts
		}
	}
	return nil
}

func isMeaningfulJSON(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	trimmed := string(raw)
	return trimmed != "null" && trimmed != `""` && trimmed != "[]"
}
