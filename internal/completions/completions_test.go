package completions

import (
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestMatchDialogs_Username(t *testing.T) {
	dialogs := []telegram.Dialog{
		{Username: "durov", Title: "Pavel Durov"},
		{Username: "telegram", Title: "Telegram"},
	}

	values := matchDialogs(dialogs, "dur")
	if len(values) != 1 || values[0] != "@durov" {
		t.Errorf("matchDialogs('dur') = %v, want [@durov]", values)
	}
}

func TestMatchDialogs_Title(t *testing.T) {
	dialogs := []telegram.Dialog{
		{Title: "My Group Chat"},
	}

	values := matchDialogs(dialogs, "my g")
	if len(values) != 1 || values[0] != "My Group Chat" {
		t.Errorf("matchDialogs('my g') = %v, want [My Group Chat]", values)
	}
}

func TestMatchDialogs_CaseInsensitive(t *testing.T) {
	dialogs := []telegram.Dialog{
		{Username: "Durov"},
	}

	values := matchDialogs(dialogs, "DUR")
	if len(values) != 1 {
		t.Errorf("matchDialogs('DUR') = %v, want 1 match", values)
	}
}

func TestMatchDialogs_Empty(t *testing.T) {
	values := matchDialogs(nil, "test")
	if len(values) != 0 {
		t.Errorf("matchDialogs(nil) = %v, want empty", values)
	}
}

func TestMatchDialogs_MaxCompletions(t *testing.T) {
	dialogs := make([]telegram.Dialog, 30)
	for idx := range dialogs {
		dialogs[idx] = telegram.Dialog{Username: "user"}
	}

	values := matchDialogs(dialogs, "u")
	if len(values) != maxCompletions {
		t.Errorf("matchDialogs returned %d, want max %d", len(values), maxCompletions)
	}
}

func TestMatchesPrefix(t *testing.T) {
	if !matchesPrefix("hello", "hel") {
		t.Error("matchesPrefix('hello', 'hel') = false")
	}

	if matchesPrefix("hello", "xyz") {
		t.Error("matchesPrefix('hello', 'xyz') = true")
	}

	if matchesPrefix("", "test") {
		t.Error("matchesPrefix('', 'test') = true")
	}
}
