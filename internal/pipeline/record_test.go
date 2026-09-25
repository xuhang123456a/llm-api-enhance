package pipeline

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-api-stronger/internal/config"
	"ai-api-stronger/internal/planner"
	"ai-api-stronger/internal/transcript"
	"ai-api-stronger/internal/upstream"
)

type capturingRecorder struct{ records []transcript.Record }

func (c *capturingRecorder) Record(record transcript.Record) { c.records = append(c.records, record) }
func (c *capturingRecorder) Close() error                    { return nil }

func (c *capturingRecorder) only(t *testing.T) transcript.Record {
	t.Helper()
	if len(c.records) != 1 {
		t.Fatalf("expected exactly 1 transcript record, got %d", len(c.records))
	}
	return c.records[0]
}

func TestPipelineEmitsTranscriptWithToolAndUsage(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"1","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`))
	}))
	defer upstreamServer.Close()

	recorder := &capturingRecorder{}
	sink := &transcript.Sink{Recorder: recorder, CaptureBodies: true, MaxBodyBytes: 4096}
	plan := testPlan(upstreamServer.URL, config.FormatOpenAI)
	plan.ToolLabel = "WorkBuddy"

	body := `{"model":"m","messages":[{"role":"user","content":"什么是闭包"}]}`
	recorderUnderTest := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/k/proxy/openai/v1/chat/completions", strings.NewReader(body))
	Pipeline{Clients: upstream.NewManager(), Transcript: sink}.Execute(recorderUnderTest, request, plan, []byte(body))

	record := recorder.only(t)
	if record.Tool != "WorkBuddy" {
		t.Fatalf("tool = %q", record.Tool)
	}
	if !strings.HasPrefix(record.SessionID, "sess-") {
		t.Fatalf("session = %q", record.SessionID)
	}
	if record.Usage == nil || record.Usage.TotalTokens != 33 {
		t.Fatalf("usage = %#v", record.Usage)
	}
	if record.Status != http.StatusOK {
		t.Fatalf("status = %d", record.Status)
	}
	if record.Model != "m" || record.Channel != config.FormatOpenAI {
		t.Fatalf("model/channel = %q/%q", record.Model, record.Channel)
	}
	if record.ResponseBody == "" || record.RequestBody != body {
		t.Fatalf("bodies not captured: req=%q resp=%q", record.RequestBody, record.ResponseBody)
	}
}

// 注入到上游的会话头必须与转录里记录的一致，否则按会话聚合会对不上。
func TestPipelineInjectsSessionHeaderMatchingTranscript(t *testing.T) {
	var seenSession string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenSession = r.Header.Get("x-opencode-session")
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstreamServer.Close()

	recorder := &capturingRecorder{}
	sink := &transcript.Sink{Recorder: recorder}
	plan := testPlan(upstreamServer.URL, config.FormatOpenAI)
	plan.RequestHeaderPlan = planner.HeaderPlan{
		Set: map[string]string{"x-opencode-session": "${session_id}"},
	}

	body := `{"model":"m","messages":[{"role":"user","content":"hello"}]}`
	Pipeline{Clients: upstream.NewManager(), Transcript: sink}.Execute(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/k/proxy/openai/v1/chat/completions", strings.NewReader(body)),
		plan,
		[]byte(body),
	)

	if seenSession == "" {
		t.Fatal("session header was not injected upstream")
	}
	if got := recorder.only(t).SessionID; got != seenSession {
		t.Fatalf("transcript session %q != injected header %q", got, seenSession)
	}
}

// 同一段对话（历史增长但首条 user 不变）必须复用同一个会话标识。
func TestPipelineKeepsSessionStableAcrossTurns(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstreamServer.Close()

	recorder := &capturingRecorder{}
	sink := &transcript.Sink{Recorder: recorder}
	plan := testPlan(upstreamServer.URL, config.FormatOpenAI)

	turnOne := `{"model":"m","messages":[{"role":"user","content":"什么是闭包"}]}`
	turnTwo := `{"model":"m","messages":[{"role":"user","content":"什么是闭包"},{"role":"assistant","content":"闭包是函数加词法作用域"},{"role":"user","content":"举个例子"}]}`
	other := `{"model":"m","messages":[{"role":"user","content":"解释事件循环"}]}`

	for _, body := range []string{turnOne, turnTwo, other} {
		Pipeline{Clients: upstream.NewManager(), Transcript: sink}.Execute(
			httptest.NewRecorder(),
			httptest.NewRequest("POST", "/k/proxy/openai/v1/chat/completions", strings.NewReader(body)),
			plan,
			[]byte(body),
		)
	}

	if len(recorder.records) != 3 {
		t.Fatalf("records = %d", len(recorder.records))
	}
	if recorder.records[0].SessionID != recorder.records[1].SessionID {
		t.Fatalf("same conversation produced different sessions: %q vs %q",
			recorder.records[0].SessionID, recorder.records[1].SessionID)
	}
	if recorder.records[0].SessionID == recorder.records[2].SessionID {
		t.Fatal("different conversations shared a session id")
	}
}

// 上游连接失败也必须留下记录，否则统计会漏掉失败的调用。
func TestPipelineRecordsUpstreamFailure(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := upstreamServer.URL
	upstreamServer.Close()

	recorder := &capturingRecorder{}
	sink := &transcript.Sink{Recorder: recorder}
	plan := testPlan(closedURL, config.FormatOpenAI)

	body := `{"model":"m","messages":[{"role":"user","content":"hi"}]}`
	Pipeline{Clients: upstream.NewManager(), Transcript: sink}.Execute(
		httptest.NewRecorder(),
		httptest.NewRequest("POST", "/k/proxy/openai/v1/chat/completions", strings.NewReader(body)),
		plan,
		[]byte(body),
	)

	record := recorder.only(t)
	if record.Error == "" {
		t.Fatal("expected an error to be recorded")
	}
	if record.Status != 0 {
		t.Fatalf("status = %d, want 0 for a connection failure", record.Status)
	}
}

// 未配置转录时不得产生任何记录，也不得影响正常转发。
func TestPipelineWithoutTranscriptSinkStillProxies(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`pong`))
	}))
	defer upstreamServer.Close()

	plan := testPlan(upstreamServer.URL, config.FormatOpenAI)
	body := `{"model":"m"}`
	responseRecorder := httptest.NewRecorder()
	Pipeline{Clients: upstream.NewManager()}.Execute(
		responseRecorder,
		httptest.NewRequest("POST", "/k/proxy/openai/v1/chat/completions", strings.NewReader(body)),
		plan,
		[]byte(body),
	)
	if responseRecorder.Body.String() != "pong" {
		t.Fatalf("body = %q", responseRecorder.Body.String())
	}
}
