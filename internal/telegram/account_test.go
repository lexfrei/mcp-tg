package telegram

import (
	"context"
	"slices"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

func TestConvertPrivacyRules(t *testing.T) {
	cases := []struct {
		name  string
		rules []tg.PrivacyRuleClass
		want  string
	}{
		{"everybody", []tg.PrivacyRuleClass{&tg.PrivacyValueAllowAll{}}, PrivacyEverybody},
		{"contacts", []tg.PrivacyRuleClass{&tg.PrivacyValueAllowContacts{}, &tg.PrivacyValueDisallowAll{}}, PrivacyContacts},
		{"nobody", []tg.PrivacyRuleClass{&tg.PrivacyValueDisallowAll{}}, PrivacyNobody},
		{"no rules", nil, PrivacyNobody},
		{"premium only", []tg.PrivacyRuleClass{&tg.PrivacyValueAllowPremium{}, &tg.PrivacyValueDisallowAll{}}, PrivacyMixed},
	}

	for _, tc := range cases {
		if got := convertPrivacyRules(tc.rules).Value; got != tc.want {
			t.Errorf("%s: value = %q, want %q", tc.name, got, tc.want)
		}
	}

	setting := convertPrivacyRules([]tg.PrivacyRuleClass{
		&tg.PrivacyValueAllowUsers{Users: []int64{1, 2}},
		&tg.PrivacyValueDisallowUsers{Users: []int64{3}},
		&tg.PrivacyValueAllowContacts{},
	})
	if !slices.Equal(setting.AllowUsers, []int64{1, 2}) || !slices.Equal(setting.DisallowUsers, []int64{3}) {
		t.Errorf("exceptions = %+v", setting)
	}
}

// Exceptions go first: Telegram applies the rules in order, so a user listed
// after disallowAll would never be reached.
func TestPrivacyRules_ExceptionsBeforeTheBase(t *testing.T) {
	rules, err := privacyRules(&PrivacyUpdate{
		Value:      PrivacyContacts,
		AllowUsers: []InputPeer{{Type: PeerUser, ID: 9, AccessHash: 1}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rules) != 3 {
		t.Fatalf("rules = %+v, want allowUsers, allowContacts, disallowAll", rules)
	}

	if _, ok := rules[0].(*tg.InputPrivacyValueAllowUsers); !ok {
		t.Errorf("first rule = %T, want the exception", rules[0])
	}

	if _, ok := rules[2].(*tg.InputPrivacyValueDisallowAll); !ok {
		t.Errorf("last rule = %T, want disallowAll closing what contacts left open", rules[2])
	}

	_, err = privacyRules(&PrivacyUpdate{Value: "friends"})
	if !errors.Is(err, ErrUnknownPrivacyValue) {
		t.Errorf("unknown value: err = %v", err)
	}
}

func TestPrivacyKeys_AreSortedAndKnown(t *testing.T) {
	keys := PrivacyKeys()
	if !slices.IsSorted(keys) || !slices.Contains(keys, "last_seen") || !slices.Contains(keys, "phone_number") {
		t.Errorf("keys = %v", keys)
	}

	_, err := (&Wrapper{}).GetPrivacy(context.Background(), "favourite_colour")
	if !errors.Is(err, ErrUnknownPrivacyKey) {
		t.Errorf("unknown key: err = %v", err)
	}
}

func TestPeerInfoFrom(t *testing.T) {
	users := []tg.UserClass{&tg.User{ID: 5, AccessHash: 7, FirstName: "Bot", Username: "helper_bot", Bot: true}}
	chats := []tg.ChatClass{&tg.Channel{ID: 8, AccessHash: 9, Title: "News", Username: "news", Broadcast: true}}

	bot, ok := peerInfoFrom(InputPeer{Type: PeerUser, ID: 5}, chats, users)
	if !ok || bot.Type != "bot" || bot.Peer.AccessHash != 7 || bot.Username != "helper_bot" {
		t.Errorf("bot = %+v", bot)
	}

	channel, ok := peerInfoFrom(InputPeer{Type: PeerChannel, ID: 8}, chats, users)
	if !ok || channel.Type != PeerInfoTypeChannel || channel.Title != "News" {
		t.Errorf("channel = %+v", channel)
	}

	if _, ok := peerInfoFrom(InputPeer{Type: PeerUser, ID: 404}, chats, users); ok {
		t.Error("a peer missing from the reply was found")
	}
}
