package relay

import (
	"strings"
	"testing"

	"telegram-notification/internal/model"
)

func TestFormatMessageContainsFields(t *testing.T) {
	t.Parallel()

	msg := formatMessage(model.NotifyRequest{
		Title:     "CPU高负载",
		Message:   "cpu > 90%",
		Level:     "critical",
		Source:    "baota",
		EventTime: "2026-04-13T09:00:00Z",
		EventID:   "evt-001",
		Labels: map[string]string{
			"host": "web-01",
		},
	})

	expectedParts := []string{
		"<b>告警通知</b>",
		"<b>标题:</b> CPU高负载",
		"<b>级别:</b> critical",
		"<b>来源:</b> baota",
		"<b>事件ID:</b> evt-001",
		"<b>内容:</b>",
		"cpu &gt; 90%",
		"- host=web-01",
	}
	for _, part := range expectedParts {
		if !strings.Contains(msg, part) {
			t.Fatalf("message should contain %q, got %s", part, msg)
		}
	}
}

func TestEscapeHTML(t *testing.T) {
	t.Parallel()

	in := `<tag>"abc"&'x'`
	out := escapeHTML(in)
	if out != "&lt;tag&gt;&quot;abc&quot;&amp;&#39;x&#39;" {
		t.Fatalf("unexpected escaped string: %s", out)
	}
}
