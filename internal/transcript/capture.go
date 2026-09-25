package transcript

import (
	"encoding/json"
	"strings"
)

// DefaultMaxBodyBytes 是单次记录中请求体/响应体各自的默认捕获上限。
const DefaultMaxBodyBytes = 256 * 1024

// tailWindowBytes 是截断后仍保留的尾部窗口大小。
// 流式响应的 usage 出现在最后一块，必须靠尾部窗口才能提取到。
const tailWindowBytes = 16 * 1024

// Capture 是一个有界写入缓冲：保留开头 limit 字节用于记录，
// 同时始终维护尾部窗口用于 token 用量提取。
type Capture struct {
	limit int
	head  []byte
	tail  []byte
	total int
}

func NewCapture(limit int) *Capture {
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}
	return &Capture{limit: limit}
}

func (c *Capture) Write(p []byte) (int, error) {
	c.total += len(p)
	if len(c.head) < c.limit {
		remaining := c.limit - len(c.head)
		if remaining > len(p) {
			remaining = len(p)
		}
		c.head = append(c.head, p[:remaining]...)
	}
	c.tail = append(c.tail, p...)
	if len(c.tail) > tailWindowBytes {
		c.tail = c.tail[len(c.tail)-tailWindowBytes:]
	}
	return len(p), nil
}

func (c *Capture) Total() int { return c.total }

// Body 返回用于记录的文本，以及是否发生了截断。
func (c *Capture) Body() (string, bool) {
	return string(c.head), c.total > len(c.head)
}

// ScanText 返回用于提取 usage 的文本：未截断时是全文，截断时退回尾部窗口。
func (c *Capture) ScanText() string {
	if c.total > len(c.head) {
		return string(c.tail)
	}
	return string(c.head)
}

// ExtractUsage 从响应文本中提取 token 用量。
// 兼容 OpenAI / Anthropic（usage 字段）与 Gemini（usageMetadata 字段），
// 以及 SSE 流式响应（取最后一个带用量的数据块）。无法识别时返回 nil。
func ExtractUsage(body string) *Usage {
	if body == "" {
		return nil
	}
	// 非流式：整段就是一个 JSON 对象。
	if usage := usageFromJSON(body); usage != nil {
		return usage
	}
	// 流式：逐行扫描 data: 前缀，保留最后一个带用量的块。
	var found *Usage
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if usage := usageFromJSON(payload); usage != nil {
			found = usage
		}
	}
	return found
}

func usageFromJSON(raw string) *Usage {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return usageFromMap(payload)
}

func usageFromMap(payload map[string]any) *Usage {
	if raw, ok := payload["usage"].(map[string]any); ok {
		usage := &Usage{
			PromptTokens:     intField(raw, "prompt_tokens", "input_tokens"),
			CompletionTokens: intField(raw, "completion_tokens", "output_tokens"),
			TotalTokens:      intField(raw, "total_tokens"),
		}
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if usage.TotalTokens == 0 {
			return nil
		}
		return usage
	}
	if raw, ok := payload["usageMetadata"].(map[string]any); ok {
		usage := &Usage{
			PromptTokens:     intField(raw, "promptTokenCount"),
			CompletionTokens: intField(raw, "candidatesTokenCount"),
			TotalTokens:      intField(raw, "totalTokenCount"),
		}
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if usage.TotalTokens == 0 {
			return nil
		}
		return usage
	}
	return nil
}

func intField(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case float64:
			return int(value)
		case int:
			return value
		}
	}
	return 0
}
