package pipeline

import (
	"io"
	"net/http"
	"time"

	"ai-api-stronger/internal/planner"
	"ai-api-stronger/internal/transcript"
)

// teeReadCloser 在读取上游响应体的同时把内容写入 capture。
type teeReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (t *teeReadCloser) Read(p []byte) (int, error) { return t.reader.Read(p) }
func (t *teeReadCloser) Close() error               { return t.closer.Close() }

// requestObservation 收集一次请求的转录上下文，并在响应写完后落盘。
type requestObservation struct {
	sink        *transcript.Sink
	plan        *planner.ExecutionPlan
	method      string
	path        string
	sessionID   string
	requestBody []byte
	started     time.Time
	capture     *transcript.Capture
}

func newRequestObservation(sink *transcript.Sink, plan *planner.ExecutionPlan, r *http.Request, requestBody []byte, sessionID string, started time.Time) *requestObservation {
	if !sink.Enabled() {
		return nil
	}
	return &requestObservation{
		sink:        sink,
		plan:        plan,
		method:      r.Method,
		path:        r.URL.Path,
		sessionID:   sessionID,
		requestBody: requestBody,
		started:     started,
		capture:     transcript.NewCapture(sink.MaxBodyBytes),
	}
}

// observe 包装上游响应体，返回一个必须在响应写完后调用的收尾函数。
// 返回的函数接收转发过程中的错误（无错误传 nil）。
func (o *requestObservation) observe(resp *http.Response, stream bool) func(error) {
	if o == nil {
		return func(error) {}
	}
	original := resp.Body
	resp.Body = &teeReadCloser{reader: io.TeeReader(original, o.capture), closer: original}
	return func(callErr error) {
		o.finish(resp.StatusCode, stream, callErr)
	}
}

func (o *requestObservation) finish(status int, stream bool, callErr error) {
	record := transcript.Record{
		RequestID:     o.plan.RequestID,
		Tool:          o.plan.ToolLabel,
		Channel:       o.plan.ChannelName,
		Model:         o.plan.ModelName,
		SessionID:     o.sessionID,
		Method:        o.method,
		Path:          o.path,
		Status:        status,
		Stream:        stream,
		ElapsedMS:     time.Since(o.started).Milliseconds(),
		RequestBytes:  len(o.requestBody),
		ResponseBytes: o.capture.Total(),
		Usage:         transcript.ExtractUsage(o.capture.ScanText()),
	}
	if callErr != nil {
		record.Error = callErr.Error()
	}
	if o.sink.CaptureBodies {
		requestBody, requestTruncated := truncateText(string(o.requestBody), o.sink.MaxBodyBytes)
		responseBody, responseTruncated := o.capture.Body()
		record.RequestBody = requestBody
		record.ResponseBody = responseBody
		record.BodyTruncated = requestTruncated || responseTruncated
	}
	o.sink.Emit(record)
}

// recordFailure 记录一次没有到达上游的失败（例如请求体变换失败）。
func (o *requestObservation) recordFailure(err error) {
	if o == nil {
		return
	}
	o.finish(0, false, err)
}

func truncateText(value string, limit int) (string, bool) {
	if limit <= 0 {
		limit = transcript.DefaultMaxBodyBytes
	}
	if len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}
