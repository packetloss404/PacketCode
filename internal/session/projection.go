package session

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/packetcode/packetcode/internal/provider"
)

const (
	// DefaultModelToolResultLimit is deliberately larger than the old 16 KiB
	// aging threshold so an immediate tool follow-up keeps useful context while
	// remaining bounded and immutable across future turns.
	DefaultModelToolResultLimit  = 64 * 1024
	minConfiguredToolResultLimit = 16 * 1024
	maxConfiguredToolResultLimit = 1024 * 1024
)

func migrateSession(s *Session, limit int) bool {
	changed := ensureModelProjections(s.Messages, limit)
	if s.FormatVersion < currentFormatVersion {
		s.FormatVersion = currentFormatVersion
		changed = true
	}
	return changed
}

func ensureModelProjections(messages []provider.Message, limit int) bool {
	changed := false
	for i := range messages {
		if ensureMessageModelProjection(&messages[i], limit) {
			changed = true
		}
	}
	return changed
}

func ensureMessageModelProjection(message *provider.Message, limit int) bool {
	if message == nil || message.Role != provider.RoleTool || message.ModelContent != "" || len(message.Content) <= limit {
		return false
	}
	message.ModelContent = canonicalToolResult(message.Content, limit)
	return true
}

func configuredModelToolResultLimit() int {
	raw := strings.TrimSpace(os.Getenv("PACKETCODE_MODEL_TOOL_RESULT_LIMIT_BYTES"))
	if raw == "" {
		return DefaultModelToolResultLimit
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < minConfiguredToolResultLimit || limit > maxConfiguredToolResultLimit {
		return DefaultModelToolResultLimit
	}
	return limit
}

// ModelMessages returns a defensive model-facing transcript. Full local/UI
// Content is replaced by the immutable projection where one is persisted, and
// the internal projection field itself is cleared before provider adapters see
// the message.
func ModelMessages(messages []provider.Message) []provider.Message {
	out := cloneMessages(messages)
	for i := range out {
		if out[i].ModelContent != "" {
			out[i].Content = out[i].ModelContent
		}
		out[i].ModelContent = ""
	}
	return out
}

func canonicalToolResult(content string, limit int) string {
	if limit <= 0 || len(content) <= limit {
		return content
	}
	markerReserve := 256
	kept := limit - markerReserve
	if kept < 2 {
		kept = 2
	}
	head := kept * 2 / 3
	tail := kept - head
	for head > 0 && !utf8.ValidString(content[:head]) {
		head--
	}
	tailStart := len(content) - tail
	for tailStart < len(content) && !utf8.RuneStart(content[tailStart]) {
		tailStart++
	}
	tail = len(content) - tailStart
	omitted := len(content) - head - tail
	return content[:head] + fmt.Sprintf("\n\n[tool result truncated: original_bytes=%d kept_head_bytes=%d kept_tail_bytes=%d omitted_bytes=%d; full result remains in session/UI]\n\n", len(content), head, tail, omitted) + content[tailStart:]
}
