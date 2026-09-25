package config

import "time"

const (
	FormatOpenAI    = "openai"
	FormatAnthropic = "anthropic"
	FormatGemini    = "gemini"

	StreamingModeNonstreamToStream = "nonstream_to_stream"
)

type Config struct {
	Server     ServerConfig             `yaml:"server" json:"server"`
	Security   SecurityConfig           `yaml:"security" json:"security"`
	Runtime    RuntimeConfig            `yaml:"runtime" json:"runtime"`
	Log        LogConfig                `yaml:"log" json:"log"`
	Transcript *TranscriptConfig        `yaml:"transcript" json:"transcript,omitempty"`
	Privacy    *LLMPrivateProtectConfig `yaml:"llm_private_protect_config" json:"llm_private_protect_config,omitempty"`
	Channels   map[string]ChannelConfig `yaml:"channels" json:"channels"`
}

// TranscriptConfig 控制请求/响应转录。
// 每条记录一行 JSON，用于回答「哪个客户端工具发了什么、花了多少 token」，
// 是横向对比场景下按工具归因的依据。默认关闭：转录会落盘完整的提示词内容。
type TranscriptConfig struct {
	Enable bool `yaml:"enable" json:"enable"`
	// Path 是 JSONL 输出路径，相对路径以进程工作目录为基准。
	Path string `yaml:"path" json:"path"`
	// CaptureBodies 为真时记录请求体与响应体原文；为假时只记录长度。
	CaptureBodies bool `yaml:"capture_bodies" json:"capture_bodies"`
	// MaxBodyBytes 限制单次记录中请求体/响应体各自的最大捕获字节数，超出部分截断。
	MaxBodyBytes int `yaml:"max_body_bytes" json:"max_body_bytes"`
}

type ServerConfig struct {
	ListenAddress    string        `yaml:"listen_address" json:"listen_address"`
	ListenPort       int           `yaml:"listen_port" json:"listen_port"`
	ReadTimeout      time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout     time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout      time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxIdleClientNum int           `yaml:"max_idle_client_num" json:"max_idle_client_num"`
}

type SecurityConfig struct {
	// AccessKey 是默认访问密钥，客户端把它填在路径第一段。
	AccessKey string `yaml:"access_key" json:"access_key"`
	// AccessKeys 允许为每个客户端工具分配独立密钥，从而在转录中按工具归因。
	// 未配置时只有 AccessKey 可用，其标签为 "default"。
	AccessKeys []AccessKeyConfig `yaml:"access_keys" json:"access_keys,omitempty"`
}

// AccessKeyConfig 把一个访问密钥映射到一个便于阅读的工具标签。
type AccessKeyConfig struct {
	Key   string `yaml:"key" json:"key"`
	Label string `yaml:"label" json:"label"`
}

type RuntimeConfig struct {
	DefaultTimeout time.Duration `yaml:"default_timeout" json:"default_timeout"`
}

type LogConfig struct {
	Enable        bool   `yaml:"enable" json:"enable"`
	Level         string `yaml:"level" json:"level"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
	Output        string `yaml:"output" json:"output"`
	Dir           string `yaml:"dir" json:"dir"`
}

type LLMPrivateProtectConfig struct {
	Model        string  `yaml:"model" json:"model"`
	BaseURL      string  `yaml:"base_url" json:"base_url"`
	APIKey       string  `yaml:"api_key" json:"api_key"`
	SystemPrompt string  `yaml:"system_prompt" json:"system_prompt"`
	Temperature  float64 `yaml:"temperature" json:"temperature"`
	Redis        string  `yaml:"redis" json:"redis"`
}

type ChannelConfig struct {
	Enabled                 bool              `yaml:"enabled" json:"enabled"`
	LLMPrivateProtectEnable bool              `yaml:"llm_private_protect_enable" json:"llm_private_protect_enable"`
	Upstream                UpstreamConfig    `yaml:"upstream" json:"upstream"`
	ChannelPolicy           PolicyConfig      `yaml:"channel_policy" json:"channel_policy"`
	ModelRules              []ModelRuleConfig `yaml:"model_rules" json:"model_rules"`
}

type UpstreamConfig struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	Format  string `yaml:"format" json:"format"`
}

type ModelRuleConfig struct {
	Model                   string       `yaml:"model" json:"model"`
	Enabled                 bool         `yaml:"enabled" json:"enabled"`
	LLMPrivateProtectEnable bool         `yaml:"llm_private_protect_enable" json:"llm_private_protect_enable"`
	Policy                  PolicyConfig `yaml:"policy" json:"policy"`
}

type PolicyConfig struct {
	Timeout          time.Duration     `yaml:"timeout" json:"timeout"`
	Proxies          []string          `yaml:"proxies" json:"proxies,omitempty"`
	RequestHeaders   HeaderMutation    `yaml:"request_headers" json:"request_headers"`
	RequestBody      RequestBodyConfig `yaml:"request_body" json:"request_body"`
	ResponseHeaders  HeaderMutation    `yaml:"response_headers" json:"response_headers"`
	ResponseStatus   map[int]int       `yaml:"response_status" json:"response_status"`
	StreamingSetting *StreamingConfig  `yaml:"streaming_setting" json:"streaming_setting,omitempty"`
}

type ProxyConfig struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type HeaderMutation struct {
	Set    map[string]string `yaml:"set" json:"set"`
	Delete []string          `yaml:"delete" json:"delete"`
}

type RequestBodyConfig struct {
	ModelRewrite map[string]string `yaml:"model_rewrite" json:"model_rewrite"`
	SystemPrompt string            `yaml:"system_prompt" json:"system_prompt"`
	MaxTokens    *int              `yaml:"max_tokens" json:"max_tokens,omitempty"`
	Temperature  *float64          `yaml:"temperature" json:"temperature,omitempty"`
	Custom       map[string]any    `yaml:"custom" json:"custom"`
}

type StreamingConfig struct {
	Mode           string        `yaml:"mode" json:"mode"`
	MaxBufferBytes int64         `yaml:"max_buffer_bytes" json:"max_buffer_bytes"`
	Timeout        time.Duration `yaml:"timeout" json:"timeout"`
}
