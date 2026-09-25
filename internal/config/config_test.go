package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBuildSnapshotDefaultsAndRules(t *testing.T) {
	yaml := []byte(`
security:
  access_key: test-access-value
channels:
  openai:
    enabled: true
    upstream:
      base_url: https://api.openai.com
      format: openai
    channel_policy:
      request_headers:
        delete: [X-Debug, X-Debug]
      request_body:
        custom:
          metadata/proxy: stronger
    model_rules:
      - model: gpt-5
        enabled: false
        policy:
          request_headers:
            set:
              X-Rule: disabled
`)
	cfg, err := LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if cfg.Runtime.DefaultTimeout == 0 {
		t.Fatalf("defaults not filled")
	}
	s, err := BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if s.ConfigHash == "" {
		t.Fatalf("empty hash")
	}
	ch := s.Channels["openai"]
	if ch == nil || !ch.Enabled {
		t.Fatalf("channel not retained")
	}
	if ch.ModelRules["gpt-5"] == nil || ch.ModelRules["gpt-5"].Enabled {
		t.Fatalf("disabled model rule not indexed as disabled")
	}
	if len(ch.ChannelPolicy.RequestHeaders.Delete) != 1 {
		t.Fatalf("header delete was not de-duped")
	}
	if got := ch.ChannelPolicy.RequestBody.CustomSet[0].Path[0]; got != "metadata" {
		t.Fatalf("custom path not compiled: %q", got)
	}
}

func TestLoadFileExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("TEST_REAL_UPSTREAM_BASE_URL", "https://example.com")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
security: {access_key: test-access-value}
channels:
  realopenai:
    enabled: true
    upstream: {base_url: ${TEST_REAL_UPSTREAM_BASE_URL}, format: openai}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := cfg.Channels["realopenai"].Upstream.BaseURL; got != "https://example.com" {
		t.Fatalf("base_url = %q, want env-expanded URL", got)
	}
}

func TestLoadFilePreservesRuntimeVariablesDuringEnvironmentExpansion(t *testing.T) {
	t.Setenv("TEST_REAL_UPSTREAM_BASE_URL", "https://example.com")
	t.Setenv("TEST_HEADER_PREFIX", "prefix")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
security: {access_key: test-access-value}
channels:
  realopenai:
    enabled: true
    upstream: {base_url: ${TEST_REAL_UPSTREAM_BASE_URL}, format: openai}
    channel_policy:
      request_headers:
        set:
          X-Trace: ${TEST_HEADER_PREFIX}-${ORIGIN}-${UUID}-${TIMESTAMP}-${uuid}
      request_body:
        custom:
          metadata/trace: ${ORIGIN}-${UUID}-${TIMESTAMP}
      response_headers:
        set:
          X-Trace: ${ORIGIN}-${UUID}-${TIMESTAMP}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := cfg.Channels["realopenai"].Upstream.BaseURL; got != "https://example.com" {
		t.Fatalf("ordinary environment variable was not expanded: %q", got)
	}
	policy := cfg.Channels["realopenai"].ChannelPolicy
	if got := policy.RequestHeaders.Set["X-Trace"]; got != "prefix-${ORIGIN}-${UUID}-${TIMESTAMP}-${uuid}" {
		t.Fatalf("request header runtime variables = %q", got)
	}
	if got := policy.RequestBody.Custom["metadata/trace"]; got != "${ORIGIN}-${UUID}-${TIMESTAMP}" {
		t.Fatalf("request body runtime variables = %q", got)
	}
	if got := policy.ResponseHeaders.Set["X-Trace"]; got != "${ORIGIN}-${UUID}-${TIMESTAMP}" {
		t.Fatalf("response header runtime variables = %q", got)
	}
}

// ${session_id} 属于运行时占位符，必须在环境变量展开阶段原样保留。
// 若被当作环境变量展开，会变成空串，请求头静默丢失（本仓库曾出现该缺陷）。
func TestLoadFilePreservesSessionIDRuntimeVariable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
security: {access_key: test-access-value}
channels:
  opencode-go:
    enabled: true
    upstream: {base_url: https://opencode.ai/zen/go, format: openai}
    channel_policy:
      request_headers:
        set:
          x-opencode-session: ${session_id}
          X-Mixed: ${session_id}-${ORIGIN}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	set := cfg.Channels["opencode-go"].ChannelPolicy.RequestHeaders.Set
	if got := set["x-opencode-session"]; got != "${session_id}" {
		t.Fatalf("session_id placeholder was not preserved: %q", got)
	}
	if got := set["X-Mixed"]; got != "${session_id}-${ORIGIN}" {
		t.Fatalf("mixed runtime variables = %q", got)
	}
}

func TestLoadEnvDoesNotOverrideExistingEnvironment(t *testing.T) {
	t.Setenv("REAL_UPSTREAM_BASE_URL", "https://existing.example")
	if err := LoadEnv([]byte("REAL_UPSTREAM_BASE_URL=https://from-file.example\nREAL_UPSTREAM_MODEL='model-name'\n")); err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if got := os.Getenv("REAL_UPSTREAM_BASE_URL"); got != "https://existing.example" {
		t.Fatalf("existing env was overwritten: %q", got)
	}
	if got := os.Getenv("REAL_UPSTREAM_MODEL"); got != "model-name" {
		t.Fatalf("quoted env value = %q, want model-name", got)
	}
}

func TestLoadBytesRejectsRemovedGlobalBodySizeFields(t *testing.T) {
	removedFields := []byte(`
security: {access_key: test-access-value}
runtime:
  default_timeout: 120s
  max_request_body_bytes: 1000
  max_response_buffer_bytes: 2000
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
`)
	_, err := LoadBytes(removedFields)
	if err == nil {
		t.Fatalf("expected removed runtime size fields to be rejected")
	}
	if !strings.Contains(err.Error(), "max_request_body_bytes") {
		t.Fatalf("expected unknown-field error for removed request size field, got %v", err)
	}
}

func TestValidateAllowsDefaultChannelAndRejectsDuplicate(t *testing.T) {
	withDefault := []byte(`
security: {access_key: test-access-value}
channels:
  default:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
`)
	if _, err := LoadBytes(withDefault); err != nil {
		t.Fatalf("default should be allowed as a normal channel name: %v", err)
	}
	dup := []byte(`
security: {access_key: test-access-value}
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
    model_rules:
      - {model: gpt-5, enabled: true}
      - {model: gpt-5, enabled: true}
`)
	if _, err := LoadBytes(dup); err == nil {
		t.Fatalf("expected duplicate model rule error")
	}
}

func TestLoadBytesRejectsLegacyProxyFieldAndInvalidProxyURL(t *testing.T) {
	legacyProxy := []byte(`
security: {access_key: test-access-value}
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
    channel_policy:
      proxy:
        type: http
        address: http://user:pass@127.0.0.1:8080
`)
	_, err := LoadBytes(legacyProxy)
	if err == nil {
		t.Fatalf("expected removed proxy field to be rejected")
	}
	if !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("expected unknown-field error for removed proxy field, got %v", err)
	}

	invalidProxy := []byte(`
security: {access_key: test-access-value}
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
    channel_policy:
      proxies:
        - ftp://127.0.0.1:8080
`)
	_, err = LoadBytes(invalidProxy)
	if err == nil {
		t.Fatalf("expected invalid proxy URL to be rejected")
	}
	if !strings.Contains(err.Error(), "proxies[0]") {
		t.Fatalf("expected proxy index in validation error, got %v", err)
	}
}

func TestBuildSnapshotCompilesProxyCandidates(t *testing.T) {
	yaml := []byte(`
security: {access_key: test-access-value}
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
    channel_policy:
      proxies:
        - http://user:pass@127.0.0.1:8080
        - socks5://127.0.0.1:1080
`)
	cfg, err := LoadBytes(yaml)
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	snapshot, err := BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	proxies := snapshot.Channels["openai"].ChannelPolicy.Proxies
	if len(proxies) != 2 {
		t.Fatalf("compiled proxies len = %d, want 2", len(proxies))
	}
	if proxies[0].Type != "http" || proxies[0].Address != "http://user:pass@127.0.0.1:8080" {
		t.Fatalf("first compiled proxy = %#v", proxies[0])
	}
	if proxies[1].Type != "socks5" || proxies[1].Address != "socks5://127.0.0.1:1080" {
		t.Fatalf("second compiled proxy = %#v", proxies[1])
	}
}

// 多访问密钥用于按客户端工具归因，必须拒绝重复密钥、重复标签和过短的密钥。
func TestValidateAccessKeys(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"duplicate key", `
security:
  access_key: test-access-value
  access_keys:
    - {key: workbuddy-key-1, label: WorkBuddy}
    - {key: workbuddy-key-1, label: Qoder}
`},
		{"duplicate label", `
security:
  access_key: test-access-value
  access_keys:
    - {key: workbuddy-key-1, label: WorkBuddy}
    - {key: qoder-key-0001, label: WorkBuddy}
`},
		{"short key", `
security:
  access_key: test-access-value
  access_keys:
    - {key: short, label: WorkBuddy}
`},
		{"missing label", `
security:
  access_key: test-access-value
  access_keys:
    - {key: workbuddy-key-1, label: "  "}
`},
		{"key shadows access_key", `
security:
  access_key: test-access-value
  access_keys:
    - {key: test-access-value, label: WorkBuddy}
`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := LoadBytes([]byte(testCase.yaml + `
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
`)); err == nil {
			t.Fatal("expected a validation error")
		}
		})
	}
}

func TestSnapshotMapsAccessKeysToToolLabels(t *testing.T) {
	cfg, err := LoadBytes([]byte(`
security:
  access_key: test-access-value
  access_keys:
    - {key: workbuddy-key-1, label: WorkBuddy}
    - {key: qoder-key-0001, label: Qoder}
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
`))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	snapshot, err := BuildSnapshot(cfg)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	for key, want := range map[string]string{
		"test-access-value": "default",
		"workbuddy-key-1":   "WorkBuddy",
		"qoder-key-0001":    "Qoder",
	} {
		got, ok := snapshot.LabelForAccessKey(key)
		if !ok || got != want {
			t.Fatalf("LabelForAccessKey(%q) = %q, %v; want %q", key, got, ok, want)
		}
	}
	if _, ok := snapshot.LabelForAccessKey("unknown-key-000"); ok {
		t.Fatal("unknown key must not resolve")
	}
}

func TestValidateTranscriptRequiresPathWhenEnabled(t *testing.T) {
	base := `
security: {access_key: test-access-value}
channels:
  openai:
    enabled: true
    upstream: {base_url: https://example.com, format: openai}
`
	if _, err := LoadBytes([]byte(base + "transcript: {enable: true}\n")); err == nil {
		t.Fatal("expected transcript.path to be required")
	}
	if _, err := LoadBytes([]byte(base + "transcript: {enable: true, path: ./t.jsonl, max_body_bytes: -1}\n")); err == nil {
		t.Fatal("expected negative max_body_bytes to be rejected")
	}
	if _, err := LoadBytes([]byte(base + "transcript: {enable: true, path: ./t.jsonl}\n")); err != nil {
		t.Fatalf("valid transcript config rejected: %v", err)
	}
	if _, err := LoadBytes([]byte(base + "transcript: {enable: false}\n")); err != nil {
		t.Fatalf("disabled transcript must not require a path: %v", err)
	}
}
