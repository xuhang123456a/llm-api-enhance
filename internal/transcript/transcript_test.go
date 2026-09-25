package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractUsageOpenAINonStream(t *testing.T) {
	body := `{"id":"1","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`
	usage := ExtractUsage(body)
	if usage == nil || usage.PromptTokens != 11 || usage.CompletionTokens != 22 || usage.TotalTokens != 33 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractUsageAnthropic(t *testing.T) {
	body := `{"id":"msg","content":[],"usage":{"input_tokens":7,"output_tokens":9}}`
	usage := ExtractUsage(body)
	if usage == nil || usage.PromptTokens != 7 || usage.CompletionTokens != 9 || usage.TotalTokens != 16 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractUsageGemini(t *testing.T) {
	body := `{"candidates":[],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":4,"totalTokenCount":7}}`
	usage := ExtractUsage(body)
	if usage == nil || usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v", usage)
	}
}

// 流式响应的用量出现在最后一个数据块，必须靠扫描 data: 行拿到。
func TestExtractUsageSSE(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"你"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"好"}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":6,"total_tokens":11}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	usage := ExtractUsage(body)
	if usage == nil || usage.TotalTokens != 11 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractUsageReturnsNilForUnknownPayload(t *testing.T) {
	if usage := ExtractUsage(`{"error":{"message":"boom"}}`); usage != nil {
		t.Fatalf("usage = %#v, want nil", usage)
	}
	if usage := ExtractUsage(""); usage != nil {
		t.Fatalf("usage = %#v, want nil", usage)
	}
}

// 超限时保留头部用于记录，同时保留尾部窗口用于用量提取。
func TestCaptureTruncatesHeadButKeepsTailForUsage(t *testing.T) {
	capture := NewCapture(16)
	// 头部远超上限，尾部是一个正常的 SSE 数据块（以换行分隔）。
	head := strings.Repeat("a", 100) + "\n"
	tail := `data: {"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}` + "\n"
	if _, err := capture.Write([]byte(head)); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Write([]byte(tail)); err != nil {
		t.Fatal(err)
	}

	body, truncated := capture.Body()
	if !truncated {
		t.Fatal("expected truncation")
	}
	if len(body) != 16 {
		t.Fatalf("captured head length = %d, want 16", len(body))
	}
	if capture.Total() != len(head)+len(tail) {
		t.Fatalf("total = %d, want %d", capture.Total(), len(head)+len(tail))
	}
	usage := ExtractUsage(capture.ScanText())
	if usage == nil || usage.TotalTokens != 3 {
		t.Fatalf("usage from tail window = %#v", usage)
	}
}

func TestCaptureWithoutTruncationScansWholeBody(t *testing.T) {
	capture := NewCapture(1024)
	body := `{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	if _, err := capture.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if _, truncated := capture.Body(); truncated {
		t.Fatal("did not expect truncation")
	}
	if capture.ScanText() != body {
		t.Fatalf("scan text = %q", capture.ScanText())
	}
}

func TestJSONLRecorderAppendsOneRecordPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "transcript.jsonl")
	recorder, err := NewJSONL(path)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	recorder.Record(Record{Tool: "WorkBuddy", Model: "deepseek-v4.1-flash"})
	recorder.Record(Record{Tool: "Qoder", Model: "deepseek-v4.1-flash"})
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	var first Record
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first.Tool != "WorkBuddy" || first.Time == "" {
		t.Fatalf("first record = %#v", first)
	}
}

// 同一路径重复打开必须追加而不是覆盖，否则重启服务会丢掉历史记录。
func TestJSONLRecorderAppendsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	for index := 0; index < 2; index++ {
		recorder, err := NewJSONL(path)
		if err != nil {
			t.Fatalf("NewJSONL: %v", err)
		}
		recorder.Record(Record{Tool: "WorkBuddy"})
		if err := recorder.Close(); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(strings.TrimSpace(string(raw)), "\n") + 1; got != 2 {
		t.Fatalf("record lines = %d, want 2", got)
	}
}

func TestNilSinkIsSafe(t *testing.T) {
	var sink *Sink
	if sink.Enabled() {
		t.Fatal("nil sink must be disabled")
	}
	sink.Emit(Record{Tool: "x"})
	if err := sink.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}
