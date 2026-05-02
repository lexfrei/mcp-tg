package prompts

import "testing"

func TestReplyPrompt_Definition(t *testing.T) {
	prompt := replyPrompt()
	if prompt.Name != "reply_to_message" {
		t.Errorf("Name = %q, want %q", prompt.Name, "reply_to_message")
	}

	if len(prompt.Arguments) != 2 {
		t.Fatalf("Arguments count = %d, want 2", len(prompt.Arguments))
	}

	if prompt.Arguments[0].Name != argPeer {
		t.Errorf("Arg[0].Name = %q, want %q", prompt.Arguments[0].Name, argPeer)
	}

	if prompt.Arguments[1].Name != "messageId" {
		t.Errorf("Arg[1].Name = %q, want %q", prompt.Arguments[1].Name, "messageId")
	}
}

func TestSummarizePrompt_Definition(t *testing.T) {
	prompt := summarizePrompt()
	if prompt.Name != "summarize_chat" {
		t.Errorf("Name = %q, want %q", prompt.Name, "summarize_chat")
	}

	if len(prompt.Arguments) != 1 {
		t.Fatalf("Arguments count = %d, want 1", len(prompt.Arguments))
	}

	if prompt.Arguments[0].Name != argPeer {
		t.Errorf("Arg[0].Name = %q, want %q", prompt.Arguments[0].Name, argPeer)
	}
}

func TestSearchAndReplyPrompt_Definition(t *testing.T) {
	prompt := searchAndReplyPrompt()
	if prompt.Name != "search_and_reply" {
		t.Errorf("Name = %q, want %q", prompt.Name, "search_and_reply")
	}

	if len(prompt.Arguments) != 2 {
		t.Fatalf("Arguments count = %d, want 2", len(prompt.Arguments))
	}

	if prompt.Arguments[0].Name != argPeer {
		t.Errorf("Arg[0].Name = %q, want %q", prompt.Arguments[0].Name, argPeer)
	}

	if prompt.Arguments[1].Name != "query" {
		t.Errorf("Arg[1].Name = %q, want %q", prompt.Arguments[1].Name, "query")
	}
}
