package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// accountInvoker answers the account requests with scripted replies and
// counts the requests that would change something.
type accountInvoker struct {
	search   *tg.ContactsFound
	contacts *tg.ContactsContacts
	imported *tg.ContactsImportedContacts
	privacy  *tg.AccountPrivacyRules
	photos   tg.PhotosPhotosClass
	writes   int
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
	case *tg.AccountSetPrivacyRequest, *tg.PhotosDeletePhotosRequest:
		a.writes++

		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
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
