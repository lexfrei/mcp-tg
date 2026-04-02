package telegram

import (
	"testing"
)

func TestNormalizePeerIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "bare username", input: "durov", want: "durov"},
		{name: "at-prefixed", input: "@durov", want: "durov"},
		{name: "https t.me link", input: "https://t.me/durov", want: "durov"},
		{name: "http t.me link", input: "http://t.me/durov", want: "durov"},
		{name: "t.me without scheme", input: "t.me/durov", want: "durov"},
		{name: "numeric stays numeric", input: "12345", want: "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePeerIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("normalizePeerIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveByID(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		wantType PeerType
		wantID   int64
	}{
		{name: "positive user ID", input: 123456, wantType: PeerUser, wantID: 123456},
		{name: "negative chat ID", input: -123456, wantType: PeerChat, wantID: 123456},
		{name: "channel ID with offset", input: -1000000000123, wantType: PeerChannel, wantID: 123},
		{name: "channel ID boundary", input: -1000000000000, wantType: PeerChannel, wantID: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveByID(tt.input)
			if got.Type != tt.wantType {
				t.Errorf("resolveByID(%d).Type = %d, want %d", tt.input, got.Type, tt.wantType)
			}

			if got.ID != tt.wantID {
				t.Errorf("resolveByID(%d).ID = %d, want %d", tt.input, got.ID, tt.wantID)
			}
		})
	}
}

func TestExtractInviteHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plus hash", input: "+abc123", want: "abc123"},
		{name: "joinchat hash", input: "joinchat/abc123", want: "abc123"},
		{name: "bare plus", input: "+", want: ""},
		{name: "bare joinchat", input: "joinchat/", want: ""},
		{name: "username", input: "durov", want: ""},
		{name: "numeric", input: "12345", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInviteHash(tt.input)
			if got != tt.want {
				t.Errorf("extractInviteHash(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
