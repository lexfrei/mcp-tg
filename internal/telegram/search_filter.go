package telegram

import (
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// Search filter names accepted by SearchOpts.Filter and
// SearchGlobalOpts.Filter. Unlike the message type labels in convert.go
// — a client-side classification applied after fetching — these map 1:1
// onto Telegram's server-side InputMessagesFilter constructors, so the
// server returns only matching messages.
// Telegram's chatPhotos and phoneCalls filters are deliberately not
// offered: they match service messages (messageActionChatEditPhoto,
// messageActionPhoneCall), which convertMessages does not surface, so
// every result page would come back empty while total reports matches.
const (
	SearchFilterPhotos     = "photos"
	SearchFilterVideo      = "video"
	SearchFilterPhotoVideo = "photo_video"
	SearchFilterDocument   = "document"
	SearchFilterURL        = "url"
	SearchFilterGif        = "gif"
	SearchFilterVoice      = "voice"
	SearchFilterMusic      = "music"
	SearchFilterRoundVoice = "round_voice"
	SearchFilterRoundVideo = "round_video"
	SearchFilterMyMentions = "my_mentions"
	SearchFilterGeo        = "geo"
	SearchFilterContacts   = "contacts"
	SearchFilterPinned     = "pinned"
	SearchFilterPoll       = "poll"
)

// Search scope names accepted by SearchGlobalOpts.Scope, restricting a
// global search to one kind of dialog. Empty means all dialogs.
const (
	SearchScopeUsers    = "users"
	SearchScopeGroups   = "groups"
	SearchScopeChannels = "channels"
)

// ErrUnknownSearchFilter is returned when a filter name is not one of
// the SearchFilter constants.
var ErrUnknownSearchFilter = errors.New("unknown search filter")

// searchFilterFactories maps filter names onto constructors of the
// corresponding TL filter. A map rather than a switch keeps the mapping
// a single lookup regardless of how many filters exist, and factories
// (rather than shared instances) keep a future conditional-flag filter
// from leaking state between calls.
func searchFilterFactories() map[string]func() tg.MessagesFilterClass {
	return map[string]func() tg.MessagesFilterClass{
		SearchFilterPhotos:     func() tg.MessagesFilterClass { return &tg.InputMessagesFilterPhotos{} },
		SearchFilterVideo:      func() tg.MessagesFilterClass { return &tg.InputMessagesFilterVideo{} },
		SearchFilterPhotoVideo: func() tg.MessagesFilterClass { return &tg.InputMessagesFilterPhotoVideo{} },
		SearchFilterDocument:   func() tg.MessagesFilterClass { return &tg.InputMessagesFilterDocument{} },
		SearchFilterURL:        func() tg.MessagesFilterClass { return &tg.InputMessagesFilterURL{} },
		SearchFilterGif:        func() tg.MessagesFilterClass { return &tg.InputMessagesFilterGif{} },
		SearchFilterVoice:      func() tg.MessagesFilterClass { return &tg.InputMessagesFilterVoice{} },
		SearchFilterMusic:      func() tg.MessagesFilterClass { return &tg.InputMessagesFilterMusic{} },
		SearchFilterRoundVoice: func() tg.MessagesFilterClass { return &tg.InputMessagesFilterRoundVoice{} },
		SearchFilterRoundVideo: func() tg.MessagesFilterClass { return &tg.InputMessagesFilterRoundVideo{} },
		SearchFilterMyMentions: func() tg.MessagesFilterClass { return &tg.InputMessagesFilterMyMentions{} },
		SearchFilterGeo:        func() tg.MessagesFilterClass { return &tg.InputMessagesFilterGeo{} },
		SearchFilterContacts:   func() tg.MessagesFilterClass { return &tg.InputMessagesFilterContacts{} },
		SearchFilterPinned:     func() tg.MessagesFilterClass { return &tg.InputMessagesFilterPinned{} },
		SearchFilterPoll:       func() tg.MessagesFilterClass { return &tg.InputMessagesFilterPoll{} },
	}
}

// SearchFilters returns the accepted filter names in sorted order, for
// validation error messages and documentation.
func SearchFilters() []string {
	factories := searchFilterFactories()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// IsSearchFilter reports whether name is one of the accepted search
// filter names. The empty string is not a filter name; callers treat it
// as "no filter" before validation.
func IsSearchFilter(name string) bool {
	_, ok := searchFilterFactories()[name]

	return ok
}

// searchFilterToTG builds the TL filter for a filter name. The empty
// name yields InputMessagesFilterEmpty, preserving the unfiltered
// behavior of searches that predate the filter parameter.
func searchFilterToTG(name string) (tg.MessagesFilterClass, error) {
	if name == "" {
		return &tg.InputMessagesFilterEmpty{}, nil
	}

	factory, ok := searchFilterFactories()[name]
	if !ok {
		return nil, errors.Wrapf(ErrUnknownSearchFilter, "%q", name)
	}

	return factory(), nil
}

// IsSearchScope reports whether scope is one of the accepted global
// search scope names. The empty string is not a scope; callers treat it
// as "all dialogs" before validation.
func IsSearchScope(scope string) bool {
	return scope == SearchScopeUsers || scope == SearchScopeGroups || scope == SearchScopeChannels
}
