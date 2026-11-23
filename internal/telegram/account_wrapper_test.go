package telegram

import (
	"bytes"
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// accountInvoker answers the account requests with scripted replies and
// records the requests that would change something, decoded from the wire
// bytes, so a test sees what the server would get.
type accountInvoker struct {
	search       *tg.ContactsFound
	contacts     *tg.ContactsContacts
	imported     *tg.ContactsImportedContacts
	privacy      *tg.AccountPrivacyRules
	photos       tg.PhotosPhotosClass
	writes       int
	setPrivacy   []*tg.AccountSetPrivacyRequest
	deletePhotos []*tg.PhotosDeletePhotosRequest
}

func (a *accountInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.ContactsSearchRequest:
		return encodeAndDecode(a.search, output)
	case *tg.ContactsGetContactsRequest:
		return encodeAndDecode(a.contacts, output)
	case *tg.ContactsImportContactsRequest:
		return encodeAndDecode(a.imported, output)
	case *tg.AccountGetPrivacyRequest:
		return encodeAndDecode(a.privacy, output)
	case *tg.PhotosGetUserPhotosRequest:
		return encodeAndDecode(a.photos, output)
	case *tg.AccountSetPrivacyRequest:
		a.writes++

		decoded := &tg.AccountSetPrivacyRequest{}
		a.setPrivacy = append(a.setPrivacy, decoded)

		return a.decodeWrite(input, decoded, &tg.AccountPrivacyRules{}, output)
	case *tg.PhotosDeletePhotosRequest:
		a.writes++

		decoded := &tg.PhotosDeletePhotosRequest{}
		a.deletePhotos = append(a.deletePhotos, decoded)

		return a.decodeWrite(input, decoded, &tg.LongVector{}, output)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
}

// decodeWrite round-trips a write request through its wire encoding into
// decoded and answers it with reply.
func (a *accountInvoker) decodeWrite(input bin.Encoder, decoded bin.Decoder, reply bin.Encoder, output bin.Decoder) error {
	err := encodeAndDecode(input, decoded)
	if err != nil {
		return err
	}

	return encodeAndDecode(reply, output)
}

func searchUser(id int64, bot bool) *tg.User {
	user := &tg.User{ID: id, AccessHash: id * 10, FirstName: "u"}
	user.SetBot(bot)

	return user
}

// The account's own matches come first, a peer in both lists appears once,
// bots are labelled, and the merged list is cut to limit — the server sends
// up to limit of each.
func TestSearchPublic_MergesDedupesAndTruncates(t *testing.T) {
	invoker := &accountInvoker{search: &tg.ContactsFound{
		MyResults: []tg.PeerClass{&tg.PeerUser{UserID: 1}, &tg.PeerUser{UserID: 2}},
		Results:   []tg.PeerClass{&tg.PeerUser{UserID: 2}, &tg.PeerUser{UserID: 3}, &tg.PeerUser{UserID: 4}},
		Users:     []tg.UserClass{searchUser(1, false), searchUser(2, true), searchUser(3, false), searchUser(4, false)},
	}}

	infos, err := newWrapperWithInvoker(invoker).SearchPublic(t.Context(), "u", 3)
	if err != nil {
		t.Fatalf("SearchPublic: %v", err)
	}

	if len(infos) != 3 || infos[0].Peer.ID != 1 || infos[1].Peer.ID != 2 || infos[2].Peer.ID != 3 {
		t.Fatalf("infos = %+v, want users 1, 2, 3", infos)
	}

	if infos[1].Type != PeerInfoTypeBot || infos[0].Type != PeerInfoTypeUser {
		t.Errorf("types = %q, %q; want user, bot", infos[0].Type, infos[1].Type)
	}
}

// Contact IDs are the only handle the contact tools return, so the replies'
// access hashes must be cached.
func TestContacts_SeedThePeerCache(t *testing.T) {
	invoker := &accountInvoker{
		contacts: &tg.ContactsContacts{Users: []tg.UserClass{searchUser(5, false)}},
		imported: &tg.ContactsImportedContacts{
			Imported: []tg.ImportedContact{{UserID: 6, ClientID: 0}},
			Users:    []tg.UserClass{searchUser(6, false)},
		},
	}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.ListContacts(t.Context())
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}

	_, err = wrap.ImportContacts(t.Context(), []PhoneContact{{Phone: "+1", FirstName: "a"}})
	if err != nil {
		t.Fatalf("ImportContacts: %v", err)
	}

	for _, id := range []int64{5, 6} {
		if cached, ok := wrap.cache.Lookup(PeerUser, id); !ok || cached.AccessHash != id*10 {
			t.Errorf("user %d: cache = %+v (%v), want its access hash", id, cached, ok)
		}
	}
}

// Every input number lands in exactly one of imported, retry and not found,
// matched by client_id.
func TestImportContacts_SortsEveryNumber(t *testing.T) {
	invoker := &accountInvoker{imported: &tg.ContactsImportedContacts{
		Imported:      []tg.ImportedContact{{UserID: 6, ClientID: 2}},
		RetryContacts: []int64{1},
		Users:         []tg.UserClass{searchUser(6, false)},
	}}

	result, err := newWrapperWithInvoker(invoker).ImportContacts(t.Context(), []PhoneContact{
		{Phone: "+10"}, {Phone: "+11"}, {Phone: "+12"},
	})
	if err != nil {
		t.Fatalf("ImportContacts: %v", err)
	}

	if len(result.Imported) != 1 || result.Imported[0] != (ImportedContact{Phone: "+12", UserID: 6}) {
		t.Errorf("imported = %+v, want +12 → 6", result.Imported)
	}

	if len(result.Retry) != 1 || result.Retry[0] != "+11" || len(result.NotFound) != 1 || result.NotFound[0] != "+10" {
		t.Errorf("retry = %v, notFound = %v; want [+11] and [+10]", result.Retry, result.NotFound)
	}
}

// The read-then-set round trip passes excepted users back by ID, so their
// access hashes must be cached on the read.
func TestGetPrivacy_SeedsTheExceptedUsers(t *testing.T) {
	invoker := &accountInvoker{privacy: &tg.AccountPrivacyRules{
		Rules: []tg.PrivacyRuleClass{&tg.PrivacyValueAllowUsers{Users: []int64{7}}, &tg.PrivacyValueDisallowAll{}},
		Users: []tg.UserClass{searchUser(7, false)},
	}}
	wrap := newWrapperWithInvoker(invoker)

	setting, err := wrap.GetPrivacy(t.Context(), "phone_number")
	if err != nil || setting.Value != PrivacyNobody {
		t.Fatalf("setting = %+v, err = %v", setting, err)
	}

	if cached, ok := wrap.cache.Lookup(PeerUser, 7); !ok || cached.AccessHash != 70 {
		t.Errorf("cache = %+v (%v), want user 7 with its hash", cached, ok)
	}
}

func TestPrivacy_RejectsAnUnknownKeyBeforeAnyRequest(t *testing.T) {
	invoker := &accountInvoker{}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.GetPrivacy(t.Context(), "shoe_size")
	if !errors.Is(err, ErrUnknownPrivacyKey) {
		t.Errorf("GetPrivacy: err = %v, want ErrUnknownPrivacyKey", err)
	}

	err = wrap.SetPrivacy(t.Context(), &PrivacyUpdate{Key: "shoe_size", Value: PrivacyNobody})
	if !errors.Is(err, ErrUnknownPrivacyKey) || invoker.writes != 0 {
		t.Errorf("SetPrivacy: err = %v, writes = %d; want ErrUnknownPrivacyKey and none", err, invoker.writes)
	}
}

func TestDeleteProfilePhoto_NoPhotoDeletesNothing(t *testing.T) {
	invoker := &accountInvoker{photos: &tg.PhotosPhotos{}}

	err := newWrapperWithInvoker(invoker).DeleteProfilePhoto(t.Context())
	if !errors.Is(err, ErrNoProfilePhoto) || invoker.writes != 0 {
		t.Errorf("err = %v, writes = %d; want ErrNoProfilePhoto and none", err, invoker.writes)
	}
}

// The delete must address photos[0] with its access hash and file
// reference: an InputPhoto with the ID alone is refused by the server.
func TestDeleteProfilePhoto_SendsTheCurrentPhoto(t *testing.T) {
	invoker := &accountInvoker{photos: &tg.PhotosPhotos{Photos: []tg.PhotoClass{
		&tg.Photo{ID: 11, AccessHash: 22, FileReference: []byte{1, 2, 3}},
		&tg.Photo{ID: 33, AccessHash: 44, FileReference: []byte{4}},
	}}}

	err := newWrapperWithInvoker(invoker).DeleteProfilePhoto(t.Context())
	if err != nil {
		t.Fatalf("DeleteProfilePhoto: %v", err)
	}

	if len(invoker.deletePhotos) != 1 || len(invoker.deletePhotos[0].ID) != 1 {
		t.Fatalf("requests = %+v, want one delete of one photo", invoker.deletePhotos)
	}

	photo, ok := invoker.deletePhotos[0].ID[0].(*tg.InputPhoto)
	if !ok {
		t.Fatalf("photo = %T, want *tg.InputPhoto", invoker.deletePhotos[0].ID[0])
	}

	if photo.ID != 11 || photo.AccessHash != 22 || !bytes.Equal(photo.FileReference, []byte{1, 2, 3}) {
		t.Errorf("photo = %+v, want photos[0]: id 11, hash 22, file reference 010203", photo)
	}
}

// Each name must reach the server as its own InputPrivacyKey — a name mapped
// onto another key would silently change a different setting — and the rules
// must be the ones privacyRules builds, exceptions included.
func TestSetPrivacy_SendsTheKeyAndRules(t *testing.T) {
	wantKeys := map[string]tg.InputPrivacyKeyClass{
		"last_seen":       &tg.InputPrivacyKeyStatusTimestamp{},
		"phone_number":    &tg.InputPrivacyKeyPhoneNumber{},
		"phone_discovery": &tg.InputPrivacyKeyAddedByPhone{},
		"profile_photo":   &tg.InputPrivacyKeyProfilePhoto{},
		"bio":             &tg.InputPrivacyKeyAbout{},
		"birthday":        &tg.InputPrivacyKeyBirthday{},
		"forwards":        &tg.InputPrivacyKeyForwards{},
		"group_invites":   &tg.InputPrivacyKeyChatInvite{},
		"calls":           &tg.InputPrivacyKeyPhoneCall{},
		"voice_messages":  &tg.InputPrivacyKeyVoiceMessages{},
	}

	if keys := PrivacyKeys(); len(keys) != len(wantKeys) {
		t.Fatalf("PrivacyKeys = %v, the table below covers %d", keys, len(wantKeys))
	}

	for _, key := range PrivacyKeys() {
		want, ok := wantKeys[key]
		if !ok {
			t.Errorf("%s: missing from the expected keys", key)

			continue
		}

		update := &PrivacyUpdate{
			Key: key, Value: PrivacyContacts,
			AllowUsers:    []InputPeer{{Type: PeerUser, ID: 7, AccessHash: 70}},
			DisallowUsers: []InputPeer{{Type: PeerUser, ID: 8, AccessHash: 80}},
		}

		invoker := &accountInvoker{}

		err := newWrapperWithInvoker(invoker).SetPrivacy(t.Context(), update)
		if err != nil || len(invoker.setPrivacy) != 1 {
			t.Fatalf("%s: err = %v, requests = %d; want one request", key, err, len(invoker.setPrivacy))
		}

		sent := invoker.setPrivacy[0]
		if sent.Key.TypeID() != want.TypeID() {
			t.Errorf("%s: key = %T, want %T", key, sent.Key, want)
		}

		wantRules, err := privacyRules(update)
		if err != nil {
			t.Fatalf("%s: privacyRules: %v", key, err)
		}

		if got, want := encodeRules(t, sent.Rules), encodeRules(t, wantRules); !bytes.Equal(got, want) {
			t.Errorf("%s: rules = %+v, want %+v", key, sent.Rules, wantRules)
		}
	}
}

func encodeRules(t *testing.T, rules []tg.InputPrivacyRuleClass) []byte {
	t.Helper()

	var buf bin.Buffer

	for _, rule := range rules {
		err := rule.Encode(&buf)
		if err != nil {
			t.Fatalf("encode %T: %v", rule, err)
		}
	}

	return buf.Raw()
}

// Basic groups and channels are separate ID spaces: a chat and a channel
// sharing an ID must each come back as itself.
func TestSearchPublic_MatchesChatHitsOnTypeAndID(t *testing.T) {
	invoker := &accountInvoker{search: &tg.ContactsFound{
		Results: []tg.PeerClass{&tg.PeerChannel{ChannelID: 5}, &tg.PeerChat{ChatID: 5}},
		Chats: []tg.ChatClass{
			&tg.Chat{ID: 5, Title: "group", Photo: &tg.ChatPhotoEmpty{}},
			&tg.Channel{ID: 5, AccessHash: 50, Title: "channel", Broadcast: true, Photo: &tg.ChatPhotoEmpty{}},
		},
	}}

	infos, err := newWrapperWithInvoker(invoker).SearchPublic(t.Context(), "x", 10)
	if err != nil {
		t.Fatalf("SearchPublic: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("infos = %+v, want the channel and the group", infos)
	}

	if infos[0].Peer.Type != PeerChannel || infos[0].Title != "channel" || infos[0].Peer.AccessHash != 50 {
		t.Errorf("first = %+v, want the channel with its access hash", infos[0])
	}

	if infos[1].Peer.Type != PeerChat || infos[1].Title != "group" {
		t.Errorf("second = %+v, want the basic group", infos[1])
	}
}

// tg_dialogs_search promises every chat the search answers with, so a basic
// group in the reply must not be dropped.
func TestSearchDialogs_KeepsBasicGroups(t *testing.T) {
	invoker := &accountInvoker{search: &tg.ContactsFound{
		Chats: []tg.ChatClass{
			&tg.Chat{ID: 5, Title: "group", Photo: &tg.ChatPhotoEmpty{}},
			&tg.Channel{ID: 6, AccessHash: 60, Title: "news", Broadcast: true, Photo: &tg.ChatPhotoEmpty{}},
		},
		Users: []tg.UserClass{searchUser(7, false)},
	}}

	dialogs, err := newWrapperWithInvoker(invoker).SearchDialogs(t.Context(), "x")
	if err != nil {
		t.Fatalf("SearchDialogs: %v", err)
	}

	var group *Dialog

	for idx := range dialogs {
		if dialogs[idx].Peer.Type == PeerChat {
			group = &dialogs[idx]
		}
	}

	if len(dialogs) != 3 || group == nil {
		t.Fatalf("dialogs = %+v, want the group, the channel and the user", dialogs)
	}

	if group.Peer.ID != 5 || group.Title != "group" || !group.IsGroup || group.IsChannel {
		t.Errorf("group = %+v, want basic group 5 titled group", group)
	}
}
