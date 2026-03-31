package resources

import "testing"

const testPeer = "durov"

func TestExtractPeer_ChatInfo(t *testing.T) {
	got := extractPeer("tg://chat/" + testPeer)
	if got != testPeer {
		t.Errorf("extractPeer(chat info) = %q, want %q", got, testPeer)
	}
}

func TestExtractPeer_ChatMessages(t *testing.T) {
	got := extractPeer("tg://chat/" + testPeer + "/messages")
	if got != testPeer {
		t.Errorf("extractPeer(chat messages) = %q, want %q", got, testPeer)
	}
}

func TestExtractPeer_WrongScheme(t *testing.T) {
	got := extractPeer("other://chat/someone")
	if got != "" {
		t.Errorf("extractPeer(wrong scheme) = %q, want empty", got)
	}
}

func TestExtractPeer_Empty(t *testing.T) {
	got := extractPeer("")
	if got != "" {
		t.Errorf("extractPeer(empty) = %q, want empty", got)
	}
}
