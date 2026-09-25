package pipeline

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	originToken    = "${ORIGIN}"
	uuidToken      = "${UUID}"
	timestampToken = "${TIMESTAMP}"
	// sessionToken 由请求体推导（见 session.go），同一段对话内保持稳定，
	// 用于满足上游「为每段对话发送稳定会话 ID」的要求。
	sessionToken = "${session_id}"
)

func interpolateRuntimeVariables(template string, origin string, now time.Time) string {
	return interpolateWithSession(template, origin, now, "")
}

// interpolateWithSession 在原有运行时变量（ORIGIN / UUID / TIMESTAMP）基础上
// 增加 ${session_id}。sessionID 为空时该占位符展开为空串，不会残留模板文本。
func interpolateWithSession(template string, origin string, now time.Time, sessionID string) string {
	var builder strings.Builder
	remaining := template
	for {
		index := nextRuntimeTokenIndex(remaining)
		if index < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		builder.WriteString(remaining[:index])
		remaining = remaining[index:]
		switch {
		case strings.HasPrefix(remaining, originToken):
			builder.WriteString(origin)
			remaining = remaining[len(originToken):]
		case strings.HasPrefix(remaining, uuidToken):
			builder.WriteString(newUUID())
			remaining = remaining[len(uuidToken):]
		case strings.HasPrefix(remaining, timestampToken):
			builder.WriteString(strconv.FormatInt(now.Unix(), 10))
			remaining = remaining[len(timestampToken):]
		case strings.HasPrefix(remaining, sessionToken):
			builder.WriteString(sessionID)
			remaining = remaining[len(sessionToken):]
		}
	}
}

func nextRuntimeTokenIndex(value string) int {
	index := -1
	for _, token := range []string{originToken, uuidToken, timestampToken, sessionToken} {
		tokenIndex := strings.Index(value, token)
		if tokenIndex >= 0 && (index < 0 || tokenIndex < index) {
			index = tokenIndex
		}
	}
	return index
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
