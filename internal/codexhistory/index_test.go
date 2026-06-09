package codexhistory

import (
	"strings"
	"testing"
)

func TestAppendConversationLineIndexesOnlyUserAndAssistantMessages(t *testing.T) {
	lines := []string{
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"permissions instructions sandbox_mode"}]}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"secret tool command"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"event stream warning exec_command"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"真实用户问题"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"真实助手回答"}]}}`,
	}
	var builder strings.Builder
	for _, line := range lines {
		appendConversationLine(&builder, []byte(line), 10_000)
	}
	got := builder.String()
	if strings.Contains(got, "permissions instructions") || strings.Contains(got, "secret tool command") || strings.Contains(got, "event stream warning") {
		t.Fatalf("indexed non-conversation content: %q", got)
	}
	if !strings.Contains(got, "真实用户问题") || !strings.Contains(got, "真实助手回答") {
		t.Fatalf("missing conversation content: %q", got)
	}
}

func TestParseThreadLineHidesDebugItemsByDefault(t *testing.T) {
	developerLine := []byte(`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"developer note"}]}}`)
	toolLine := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"ls"}}`)
	userLine := []byte(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`)

	if _, ok := parseThreadLine(developerLine, 10_000, false); ok {
		t.Fatal("developer message should be hidden by default")
	}
	if _, ok := parseThreadLine(toolLine, 10_000, false); ok {
		t.Fatal("tool call should be hidden by default")
	}
	if item, ok := parseThreadLine(userLine, 10_000, false); !ok || item.Text != "hello" {
		t.Fatalf("user message should be visible, got ok=%v item=%+v", ok, item)
	}
	if _, ok := parseThreadLine(toolLine, 10_000, true); !ok {
		t.Fatal("tool call should be visible in debug mode")
	}
}
