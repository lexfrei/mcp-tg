package telegram

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// Privacy values. PrivacyMixed is read-only: a rule set this server cannot
// express as one of the others (bots, premium users, chat members).
const (
	PrivacyEverybody    = "everybody"
	PrivacyContacts     = "contacts"
	PrivacyCloseFriends = "close_friends"
	PrivacyNobody       = "nobody"
	PrivacyMixed        = "mixed"
)

// ErrUnknownPrivacyKey is returned for a privacy setting name not in PrivacyKeys.
var ErrUnknownPrivacyKey = errors.New("unknown privacy setting")

// ErrUnknownPrivacyValue is returned for a value other than everybody,
// contacts, close_friends or nobody.
var ErrUnknownPrivacyValue = errors.New("privacy value must be everybody, contacts, close_friends or nobody")

// ErrNoProfilePhoto is returned when there is no profile photo to delete.
var ErrNoProfilePhoto = errors.New("the account has no profile photo")

// privacyKeys maps the names the tools accept onto Telegram's privacy keys.
//
//nolint:gochecknoglobals // read-only lookup table, like searchFilters.
var privacyKeys = map[string]func() tg.InputPrivacyKeyClass{
	"last_seen":       func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyStatusTimestamp{} },
	"phone_number":    func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyPhoneNumber{} },
	"phone_discovery": func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyAddedByPhone{} },
	"profile_photo":   func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyProfilePhoto{} },
	"bio":             func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyAbout{} },
	"birthday":        func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyBirthday{} },
	"forwards":        func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyForwards{} },
	"group_invites":   func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyChatInvite{} },
	"calls":           func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyPhoneCall{} },
	"voice_messages":  func() tg.InputPrivacyKeyClass { return &tg.InputPrivacyKeyVoiceMessages{} },
}

// PrivacyKeys returns the privacy setting names, sorted.
func PrivacyKeys() []string {
	return slices.Sorted(maps.Keys(privacyKeys))
}

// PrivacySetting is one privacy setting: who may see or do the thing, plus
// the users excepted either way.
type PrivacySetting struct {
	Key           string  `json:"key"`
	Value         string  `json:"value"`
	AllowUsers    []int64 `json:"allowUsers,omitempty"`
	DisallowUsers []int64 `json:"disallowUsers,omitempty"`
}

// PrivacyUpdate is a privacy setting to store; the exceptions are resolved
// peers rather than bare IDs, since the request needs their access hashes.
type PrivacyUpdate struct {
	Key           string
	Value         string
	AllowUsers    []InputPeer
	DisallowUsers []InputPeer
}

// PhoneContact is a contact to import by phone number.
type PhoneContact struct {
	Phone     string
	FirstName string
	LastName  string
}

// ImportResult reports which imported numbers belong to Telegram users.
type ImportResult struct {
	Users   []User `json:"users"`
	Skipped int    `json:"skipped"`
}

// GetPrivacy reads one privacy setting.
func (w *Wrapper) GetPrivacy(ctx context.Context, key string) (*PrivacySetting, error) {
	build, ok := privacyKeys[key]
	if !ok {
		return nil, errors.Wrapf(ErrUnknownPrivacyKey, "%q (one of %s)", key, strings.Join(PrivacyKeys(), ", "))
	}

	rules, err := w.api.AccountGetPrivacy(ctx, build())
	if err != nil {
		return nil, errors.Wrap(err, "reading privacy setting")
	}

	setting := convertPrivacyRules(rules.Rules)
	setting.Key = key

	return setting, nil
}

// SetPrivacy stores one privacy setting.
func (w *Wrapper) SetPrivacy(ctx context.Context, update *PrivacyUpdate) error {
	build, ok := privacyKeys[update.Key]
	if !ok {
		return errors.Wrapf(ErrUnknownPrivacyKey, "%q (one of %s)", update.Key, strings.Join(PrivacyKeys(), ", "))
	}

	rules, err := privacyRules(update)
	if err != nil {
		return err
	}

	_, err = w.api.AccountSetPrivacy(ctx, &tg.AccountSetPrivacyRequest{Key: build(), Rules: rules})
	if err != nil {
		return errors.Wrap(err, "storing privacy setting")
	}

	return nil
}

// DeleteProfilePhoto deletes the account's current profile photo. The
// previous one, if any, becomes current, as in the official clients.
func (w *Wrapper) DeleteProfilePhoto(ctx context.Context) error {
	result, err := w.api.PhotosGetUserPhotos(ctx, &tg.PhotosGetUserPhotosRequest{UserID: &tg.InputUserSelf{}, Limit: 1})
	if err != nil {
		return errors.Wrap(err, "reading profile photos")
	}

	photos := result.GetPhotos()
	if len(photos) == 0 {
		return ErrNoProfilePhoto
	}

	photo, ok := photos[0].(*tg.Photo)
	if !ok {
		return ErrNoProfilePhoto
	}

	_, err = w.api.PhotosDeletePhotos(ctx, []tg.InputPhotoClass{
		&tg.InputPhoto{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference},
	})
	if err != nil {
		return errors.Wrap(err, "deleting profile photo")
	}

	return nil
}

// SearchPublic searches public chats, channels, bots and users by name or
// username, beyond the account's own dialogs.
func (w *Wrapper) SearchPublic(ctx context.Context, query string, limit int) ([]PeerInfo, error) {
	found, err := w.api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: query, Limit: limit})
	if err != nil {
		return nil, errors.Wrap(err, "searching public chats")
	}

	w.cachePeersOf(found.Chats, found.Users)

	peers := slices.Concat(found.MyResults, found.Results)
	infos := make([]PeerInfo, 0, len(peers))
	seen := make(map[peerKey]bool, len(peers))

	for _, peer := range peers {
		info, ok := peerInfoFrom(extractPeerID(peer), found.Chats, found.Users)
		key := peerKey{typ: info.Peer.Type, id: info.Peer.ID}

		if ok && !seen[key] {
			seen[key] = true

			infos = append(infos, info)
		}
	}

	// The reply holds two lists, the account's own matches and global ones,
	// each up to limit long — verified live, limit 3 answered with 6 peers.
	if len(infos) > limit {
		infos = infos[:limit]
	}

	return infos, nil
}

// ListContacts returns the account's whole contact list.
func (w *Wrapper) ListContacts(ctx context.Context) ([]User, error) {
	result, err := w.api.ContactsGetContacts(ctx, 0)
	if err != nil {
		return nil, errors.Wrap(err, "listing contacts")
	}

	contacts, ok := result.(*tg.ContactsContacts)
	if !ok {
		return nil, nil
	}

	return convertUsers(contacts.Users), nil
}

// ImportContacts adds contacts by phone number.
func (w *Wrapper) ImportContacts(ctx context.Context, contacts []PhoneContact) (*ImportResult, error) {
	input := make([]tg.InputPhoneContact, len(contacts))
	for idx, contact := range contacts {
		input[idx] = tg.InputPhoneContact{
			ClientID:  int64(idx),
			Phone:     contact.Phone,
			FirstName: contact.FirstName,
			LastName:  contact.LastName,
		}
	}

	result, err := w.api.ContactsImportContacts(ctx, input)
	if err != nil {
		return nil, errors.Wrap(err, "importing contacts")
	}

	users := convertUsers(result.Users)

	return &ImportResult{Users: users, Skipped: len(contacts) - len(result.Imported)}, nil
}

// ReorderFolders sets the order of the account's chat folders.
func (w *Wrapper) ReorderFolders(ctx context.Context, folderIDs []int) error {
	_, err := w.api.MessagesUpdateDialogFiltersOrder(ctx, folderIDs)
	if err != nil {
		return errors.Wrap(err, "reordering folders")
	}

	return nil
}

func convertPrivacyRules(rules []tg.PrivacyRuleClass) *PrivacySetting {
	setting := &PrivacySetting{Value: PrivacyNobody}
	base := ""

	for _, rule := range rules {
		switch typed := rule.(type) {
		case *tg.PrivacyValueAllowUsers:
			setting.AllowUsers = append(setting.AllowUsers, typed.Users...)
		case *tg.PrivacyValueDisallowUsers:
			setting.DisallowUsers = append(setting.DisallowUsers, typed.Users...)
		case *tg.PrivacyValueAllowAll:
			base = firstBase(base, PrivacyEverybody)
		case *tg.PrivacyValueAllowContacts:
			base = firstBase(base, PrivacyContacts)
		case *tg.PrivacyValueAllowCloseFriends:
			base = firstBase(base, PrivacyCloseFriends)
		case *tg.PrivacyValueDisallowAll:
			base = firstBase(base, PrivacyNobody)
		default:
			base = firstBase(base, PrivacyMixed)
		}
	}

	if base != "" {
		setting.Value = base
	}

	return setting
}

// firstBase keeps the first base rule: Telegram applies rules in order, so a
// later disallowAll after allowContacts only closes what contacts left open.
func firstBase(current, next string) string {
	if current != "" {
		return current
	}

	return next
}

func privacyRules(update *PrivacyUpdate) ([]tg.InputPrivacyRuleClass, error) {
	var rules []tg.InputPrivacyRuleClass

	if len(update.AllowUsers) > 0 {
		rules = append(rules, &tg.InputPrivacyValueAllowUsers{Users: inputUsers(update.AllowUsers)})
	}

	if len(update.DisallowUsers) > 0 {
		rules = append(rules, &tg.InputPrivacyValueDisallowUsers{Users: inputUsers(update.DisallowUsers)})
	}

	switch update.Value {
	case PrivacyEverybody:
		rules = append(rules, &tg.InputPrivacyValueAllowAll{})
	case PrivacyContacts:
		rules = append(rules, &tg.InputPrivacyValueAllowContacts{}, &tg.InputPrivacyValueDisallowAll{})
	case PrivacyCloseFriends:
		rules = append(rules, &tg.InputPrivacyValueAllowCloseFriends{}, &tg.InputPrivacyValueDisallowAll{})
	case PrivacyNobody:
		rules = append(rules, &tg.InputPrivacyValueDisallowAll{})
	default:
		return nil, errors.Wrapf(ErrUnknownPrivacyValue, "%q", update.Value)
	}

	return rules, nil
}

func inputUsers(peers []InputPeer) []tg.InputUserClass {
	users := make([]tg.InputUserClass, len(peers))
	for idx, peer := range peers {
		users[idx] = InputUserFromPeer(peer)
	}

	return users
}

func convertUsers(classes []tg.UserClass) []User {
	users := make([]User, 0, len(classes))

	for _, class := range classes {
		if typed, ok := class.(*tg.User); ok {
			users = append(users, ConvertUser(typed))
		}
	}

	return users
}

// peerInfoFrom builds the PeerInfo of a search hit from the chats and users
// the reply carried.
func peerInfoFrom(peer InputPeer, chats []tg.ChatClass, users []tg.UserClass) (PeerInfo, bool) {
	if peer.Type == PeerUser {
		for _, class := range users {
			if user, ok := class.(*tg.User); ok && user.ID == peer.ID {
				kind := PeerInfoTypeUser
				if user.Bot {
					kind = "bot"
				}

				return PeerInfo{
					Peer:     InputPeer{Type: PeerUser, ID: user.ID, AccessHash: user.AccessHash},
					Title:    userDisplayName(user),
					Username: user.Username,
					Type:     kind,
				}, true
			}
		}

		return PeerInfo{}, false
	}

	for _, class := range chats {
		info, ok := searchChatInfo(class)
		if ok && info.Peer.ID == peer.ID {
			return info, true
		}
	}

	return PeerInfo{}, false
}

func searchChatInfo(class tg.ChatClass) (PeerInfo, bool) {
	switch typed := class.(type) {
	case *tg.Channel:
		kind := PeerInfoTypeSupergroup
		if typed.Broadcast {
			kind = PeerInfoTypeChannel
		}

		return PeerInfo{
			Peer:     InputPeer{Type: PeerChannel, ID: typed.ID, AccessHash: typed.AccessHash},
			Title:    typed.Title,
			Username: typed.Username,
			Type:     kind,
		}, true
	case *tg.Chat:
		return PeerInfo{Peer: InputPeer{Type: PeerChat, ID: typed.ID}, Title: typed.Title, Type: PeerInfoTypeChat}, true
	default:
		return PeerInfo{}, false
	}
}
