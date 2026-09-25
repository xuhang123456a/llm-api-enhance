// Package transcript 记录代理转发的每一次请求与响应。
//
// 存在的理由：当多个客户端工具共用一个代理时，上游只看到「一个客户端」，
// 无法回答「哪个工具发了什么、花了多少 token」。本包把每次请求的
// 工具标签、会话标识、模型、耗时与 token 用量落成 JSONL，
// 使按工具归因成为可能。
package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Usage 是归一化后的 token 用量。不同上游格式字段名不同，统一到这里。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Record 是一条转录记录，一行 JSON。
type Record struct {
	Time          string `json:"time"`
	RequestID     string `json:"request_id"`
	Tool          string `json:"tool"`
	Channel       string `json:"channel"`
	Model         string `json:"model"`
	SessionID     string `json:"session_id"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Status        int    `json:"status"`
	Stream        bool   `json:"stream"`
	ElapsedMS     int64  `json:"elapsed_ms"`
	Error         string `json:"error,omitempty"`
	Usage         *Usage `json:"usage,omitempty"`
	RequestBytes  int    `json:"request_bytes"`
	ResponseBytes int    `json:"response_bytes"`
	// RequestBody / ResponseBody 仅在 capture_bodies 打开时出现。
	RequestBody   string `json:"request_body,omitempty"`
	ResponseBody  string `json:"response_body,omitempty"`
	BodyTruncated bool   `json:"body_truncated,omitempty"`
}

// Recorder 是转录的落盘出口。
type Recorder interface {
	Record(Record)
	Close() error
}

// Sink 把配置与输出目标绑在一起，供 pipeline 直接使用。
type Sink struct {
	Recorder      Recorder
	CaptureBodies bool
	MaxBodyBytes  int
}

func (s *Sink) Enabled() bool { return s != nil && s.Recorder != nil }

// Finish 在 Sink 为 nil 时安全地做收尾。
func (s *Sink) Finish() error {
	if s == nil || s.Recorder == nil {
		return nil
	}
	return s.Recorder.Close()
}

func (s *Sink) Emit(record Record) {
	if s == nil || s.Recorder == nil {
		return
	}
	s.Recorder.Record(record)
}

type noopRecorder struct{}

func NewNoop() Recorder                    { return noopRecorder{} }
func (noopRecorder) Record(Record)         {}
func (noopRecorder) Close() error          { return nil }

// JSONLRecorder 把记录逐行追加写入文件。
type JSONLRecorder struct {
	mu   sync.Mutex
	file *os.File
}

// NewJSONL 打开（必要时创建）JSONL 输出文件。文件以追加模式打开，
// 多次启动服务不会覆盖历史记录。
func NewJSONL(path string) (*JSONLRecorder, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &JSONLRecorder{file: file}, nil
}

func (r *JSONLRecorder) Record(record Record) {
	if record.Time == "" {
		record.Time = time.Now().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	_, _ = r.file.Write(append(line, '\n'))
}

func (r *JSONLRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
