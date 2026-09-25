package pipeline

import (
	"testing"
	"time"
)

func TestSessionIDStableWhenHistoryGrows(t *testing.T) {
	first := []byte(`{"model":"m","messages":[{"role":"user","content":"写一个快速排序"}]}`)
	second := []byte(`{"model":"m","messages":[
		{"role":"user","content":"写一个快速排序"},
		{"role":"assistant","content":"好的，这是实现……"},
		{"role":"user","content":"再加上中文注释"}
	]}`)

	if SessionIDFromBody(first) == "" {
		t.Fatal("会话 ID 不应为空")
	}
	if SessionIDFromBody(first) != SessionIDFromBody(second) {
		t.Fatalf("同一段对话在历史增长后应保持同一个会话 ID：%s vs %s",
			SessionIDFromBody(first), SessionIDFromBody(second))
	}
}

func TestSessionIDDiffersBetweenConversations(t *testing.T) {
	a := []byte(`{"messages":[{"role":"user","content":"任务 A"}]}`)
	b := []byte(`{"messages":[{"role":"user","content":"任务 B"}]}`)
	if SessionIDFromBody(a) == SessionIDFromBody(b) {
		t.Fatal("不同对话应得到不同的会话 ID")
	}
}

func TestSessionIDIgnoresLeadingSystemMessage(t *testing.T) {
	withSystem := []byte(`{"messages":[
		{"role":"system","content":"你是一个编程助手"},
		{"role":"user","content":"修复这个 bug"}
	]}`)
	withoutSystem := []byte(`{"messages":[{"role":"user","content":"修复这个 bug"}]}`)

	if SessionIDFromBody(withSystem) != SessionIDFromBody(withoutSystem) {
		t.Fatal("system 提示词的有无不应影响会话 ID，它应由第一条 user 消息决定")
	}
}

func TestSessionIDSupportsAnthropicContentBlocks(t *testing.T) {
	anthropic := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	if SessionIDFromBody(anthropic) == "" {
		t.Fatal("Anthropic 内容块格式应能解析出会话 ID")
	}
}

func TestSessionIDSupportsGeminiContents(t *testing.T) {
	gemini := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	if SessionIDFromBody(gemini) == "" {
		t.Fatal("Gemini contents 格式应能解析出会话 ID")
	}
}

func TestSessionIDFallsBackOnUnparsableBody(t *testing.T) {
	if SessionIDFromBody([]byte("not json at all")) == "" {
		t.Fatal("无法解析的请求体应退化为整体哈希，而不是返回空值")
	}
	if SessionIDFromBody(nil) != "" {
		t.Fatal("空请求体应返回空字符串")
	}
}

func TestSessionIDHasPrefix(t *testing.T) {
	id := SessionIDFromBody([]byte(`{"messages":[{"role":"user","content":"x"}]}`))
	if len(id) <= len(sessionIDPrefix) || id[:len(sessionIDPrefix)] != sessionIDPrefix {
		t.Fatalf("会话 ID 应带有 %s 前缀，实际为 %s", sessionIDPrefix, id)
	}
}

func TestInterpolateSessionToken(t *testing.T) {
	got := interpolateWithSession("Bearer ${session_id}", "", time.Now(), "sess-abc123")
	if got != "Bearer sess-abc123" {
		t.Fatalf("${session_id} 未被正确替换：%s", got)
	}
}

func TestInterpolateSessionTokenWithoutSession(t *testing.T) {
	got := interpolateWithSession("${session_id}", "", time.Now(), "")
	if got != "" {
		t.Fatalf("会话为空时不应残留模板文本，实际为 %q", got)
	}
}
