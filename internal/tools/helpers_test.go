package tools

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestDeref_NilInt(t *testing.T) {
	got := deref[int](nil)

	if got != 0 {
		t.Errorf("deref(nil *int) = %d, want 0", got)
	}
}

func TestDeref_Int(t *testing.T) {
	val := 5
	got := deref(&val)

	if got != 5 {
		t.Errorf("deref(&5) = %d, want 5", got)
	}
}

func TestDeref_NilString(t *testing.T) {
	got := deref[string](nil)

	if got != "" {
		t.Errorf("deref(nil *string) = %q, want %q", got, "")
	}
}

func TestDeref_String(t *testing.T) {
	val := "test_value"
	got := deref(&val)

	if got != val {
		t.Errorf("deref(&string) = %q, want %q", got, val)
	}
}

func TestDeref_NilBool(t *testing.T) {
	got := deref[bool](nil)

	if got {
		t.Error("deref(nil *bool) = true, want false")
	}
}

func TestFormatPeer_User(t *testing.T) {
	peer := telegram.InputPeer{Type: telegram.PeerUser, ID: 123}
	got := formatPeer(peer)
	want := "123"

	if got != want {
		t.Errorf("formatPeer(user) = %q, want %q", got, want)
	}
}

func TestFormatPeer_Chat(t *testing.T) {
	peer := telegram.InputPeer{Type: telegram.PeerChat, ID: 456}
	got := formatPeer(peer)
	want := "-456"

	if got != want {
		t.Errorf("formatPeer(chat) = %q, want %q", got, want)
	}
}

func TestFormatPeer_Channel(t *testing.T) {
	peer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 789}
	got := formatPeer(peer)
	want := "-1000000000789"

	if got != want {
		t.Errorf("formatPeer(channel) = %q, want %q", got, want)
	}
}

func TestFormatPeer_Unknown(t *testing.T) {
	peer := telegram.InputPeer{Type: 99, ID: 111}
	got := formatPeer(peer)
	want := "111"

	if got != want {
		t.Errorf("formatPeer(unknown) = %q, want %q", got, want)
	}
}

func TestFormatUserName_Nil(t *testing.T) {
	got := formatUserName(nil)

	if got != "unknown" {
		t.Errorf("formatUserName(nil) = %q, want %q", got, "unknown")
	}
}

func TestFormatUserName_FullName(t *testing.T) {
	user := &telegram.User{FirstName: "John", LastName: "Doe"}
	got := formatUserName(user)
	want := "John Doe"

	if got != want {
		t.Errorf("formatUserName() = %q, want %q", got, want)
	}
}

func TestFormatUserName_FirstOnly(t *testing.T) {
	user := &telegram.User{FirstName: "John"}
	got := formatUserName(user)
	want := "John"

	if got != want {
		t.Errorf("formatUserName() = %q, want %q", got, want)
	}
}

func TestFormatUserName_WithUsername(t *testing.T) {
	user := &telegram.User{
		FirstName: "John",
		LastName:  "Doe",
		Username:  "johndoe",
	}
	got := formatUserName(user)
	want := "John Doe (@johndoe)"

	if got != want {
		t.Errorf("formatUserName() = %q, want %q", got, want)
	}
}

func TestValidateIDCount_OK(t *testing.T) {
	ids := make([]int, maxIDsPerRequest)

	if validateIDCount(ids) != nil {
		t.Errorf("expected nil for %d IDs", maxIDsPerRequest)
	}
}

func TestValidateIDCount_TooMany(t *testing.T) {
	ids := make([]int, maxIDsPerRequest+1)

	if validateIDCount(ids) == nil {
		t.Error("expected error for too many IDs")
	}
}

func TestValidateIDCount_Empty(t *testing.T) {
	if validateIDCount(nil) != nil {
		t.Error("expected nil for empty IDs")
	}
}

const truncateHello = "hello"

func TestTruncateText_Short(t *testing.T) {
	got := truncateText(truncateHello, 10)

	if got != truncateHello {
		t.Errorf("truncateText = %q, want %q", got, truncateHello)
	}
}

func TestTruncateText_ExactLength(t *testing.T) {
	got := truncateText(truncateHello, 5)

	if got != truncateHello {
		t.Errorf("truncateText at exact length = %q, want no ellipsis", got)
	}
}

func TestTruncateText_Long(t *testing.T) {
	got := truncateText("helloworld", 5)

	if got != "hello…" {
		t.Errorf("truncateText = %q, want %q", got, "hello…")
	}
}

func TestTruncateText_UTF8(t *testing.T) {
	// Кириллица — 2 байта на рун, не должно биться по байтам.
	got := truncateText("привет мир", 6)

	if got != "привет…" {
		t.Errorf("truncateText (UTF-8) = %q, want %q", got, "привет…")
	}
}

func TestTruncateText_Emoji(t *testing.T) {
	got := truncateText("ab🙂cd", 3)

	if got != "ab🙂…" {
		t.Errorf("truncateText (emoji) = %q, want %q", got, "ab🙂…")
	}
}

func TestTruncateText_ZeroMax(t *testing.T) {
	got := truncateText("anything", 0)

	if got != "" {
		t.Errorf("truncateText with max=0 = %q, want empty", got)
	}
}

func TestValidateParseMode_Allowed(t *testing.T) {
	cases := []string{"", "markdown", "commonmark", "Markdown", "COMMONMARK"}

	for _, mode := range cases {
		err := validateParseMode(mode)
		if err != nil {
			t.Errorf("validateParseMode(%q) = %v, want nil", mode, err)
		}
	}
}

func TestValidateParseMode_NotImplemented(t *testing.T) {
	cases := []string{"html", "markdownv2", "HTML", "MarkdownV2"}

	for _, mode := range cases {
		err := validateParseMode(mode)
		if !errors.Is(err, ErrParseModeNotImplemented) {
			t.Errorf("validateParseMode(%q) = %v, want ErrParseModeNotImplemented", mode, err)
		}
	}
}

func TestValidateParseMode_Unknown(t *testing.T) {
	cases := []string{"md", "whatever", "rich"}

	for _, mode := range cases {
		err := validateParseMode(mode)
		if !errors.Is(err, ErrUnknownParseMode) {
			t.Errorf("validateParseMode(%q) = %v, want ErrUnknownParseMode", mode, err)
		}
	}
}

func TestTruncateText_NegativeMax(t *testing.T) {
	got := truncateText("anything", -1)

	if got != "" {
		t.Errorf("truncateText with negative max = %q, want empty", got)
	}
}

func TestFormatPeerRef_WithUsername(t *testing.T) {
	got := formatPeerRef("Alice", "alice", telegram.InputPeer{Type: telegram.PeerUser, ID: 10})
	want := "Alice [@alice]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestFormatPeerRef_IDOnlyUser(t *testing.T) {
	got := formatPeerRef("Bob", "", telegram.InputPeer{Type: telegram.PeerUser, ID: 99})
	want := "Bob [user:99]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestFormatPeerRef_ChannelLabel(t *testing.T) {
	got := formatPeerRef("Cozystack Blog", "cozystack_blog",
		telegram.InputPeer{Type: telegram.PeerChannel, ID: 500})
	want := "Cozystack Blog [@cozystack_blog]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestFormatPeerRef_PrivateChannelID(t *testing.T) {
	got := formatPeerRef("Private", "", telegram.InputPeer{Type: telegram.PeerChannel, ID: 500})
	want := "Private [channel:500]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestFormatPeerRef_HiddenPrivacy(t *testing.T) {
	got := formatPeerRef("Kaidxen", "", telegram.InputPeer{})
	want := "Kaidxen [hidden]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestFormatPeerRef_AllUnknown(t *testing.T) {
	got := formatPeerRef("", "", telegram.InputPeer{})
	want := "[hidden]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestFormatPeerRef_LabelOnly(t *testing.T) {
	got := formatPeerRef("", "alice", telegram.InputPeer{Type: telegram.PeerUser, ID: 10})
	want := "[@alice]"

	if got != want {
		t.Errorf("formatPeerRef = %q, want %q", got, want)
	}
}

func TestPeerLabel_BasicGroupChat(t *testing.T) {
	got := peerLabel(telegram.InputPeer{Type: telegram.PeerChat, ID: 77}, "")
	want := "group:77"

	if got != want {
		t.Errorf("peerLabel = %q, want %q — matches participant.type and fromType label", got, want)
	}
}

func TestPeerLabel_UnknownTypeSurfacesAsUnknown(t *testing.T) {
	// PeerType is the Go iota enum; a future API extension or a
	// directly-constructed struct could carry an out-of-range value.
	// The default branch must NOT silently relabel it as "user:N".
	got := peerLabel(telegram.InputPeer{Type: telegram.PeerType(99), ID: 42}, "")
	want := "unknown:42"

	if got != want {
		t.Errorf("peerLabel for unknown PeerType = %q, want %q — must not masquerade as user:", got, want)
	}
}
