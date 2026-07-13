package telegram

import (
	"reflect"
	"slices"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

func searchFilterCases() map[string]tg.MessagesFilterClass {
	return map[string]tg.MessagesFilterClass{
		SearchFilterPhotos:      &tg.InputMessagesFilterPhotos{},
		SearchFilterVideo:       &tg.InputMessagesFilterVideo{},
		SearchFilterPhotoVideo:  &tg.InputMessagesFilterPhotoVideo{},
		SearchFilterDocument:    &tg.InputMessagesFilterDocument{},
		SearchFilterURL:         &tg.InputMessagesFilterURL{},
		SearchFilterGif:         &tg.InputMessagesFilterGif{},
		SearchFilterVoice:       &tg.InputMessagesFilterVoice{},
		SearchFilterMusic:       &tg.InputMessagesFilterMusic{},
		SearchFilterChatPhotos:  &tg.InputMessagesFilterChatPhotos{},
		SearchFilterRoundVoice:  &tg.InputMessagesFilterRoundVoice{},
		SearchFilterRoundVideo:  &tg.InputMessagesFilterRoundVideo{},
		SearchFilterMyMentions:  &tg.InputMessagesFilterMyMentions{},
		SearchFilterGeo:         &tg.InputMessagesFilterGeo{},
		SearchFilterContacts:    &tg.InputMessagesFilterContacts{},
		SearchFilterPinned:      &tg.InputMessagesFilterPinned{},
		SearchFilterPoll:        &tg.InputMessagesFilterPoll{},
		SearchFilterPhoneCalls:  &tg.InputMessagesFilterPhoneCalls{},
		SearchFilterMissedCalls: &tg.InputMessagesFilterPhoneCalls{},
	}
}

func TestSearchFilterToTG_MapsEveryName(t *testing.T) {
	for name, want := range searchFilterCases() {
		got, err := searchFilterToTG(name)
		if err != nil {
			t.Errorf("searchFilterToTG(%q) returned error: %v", name, err)

			continue
		}

		if reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Errorf("searchFilterToTG(%q) = %T, want %T", name, got, want)
		}
	}
}

func TestSearchFilterToTG_MissedCallsSetsMissed(t *testing.T) {
	got, err := searchFilterToTG(SearchFilterMissedCalls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls, ok := got.(*tg.InputMessagesFilterPhoneCalls)
	if !ok {
		t.Fatalf("got %T, want *tg.InputMessagesFilterPhoneCalls", got)
	}

	if !calls.GetMissed() {
		t.Error("missed_calls filter must set the Missed flag")
	}
}

func TestSearchFilterToTG_PhoneCallsKeepsMissedClear(t *testing.T) {
	got, err := searchFilterToTG(SearchFilterPhoneCalls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls, ok := got.(*tg.InputMessagesFilterPhoneCalls)
	if !ok {
		t.Fatalf("got %T, want *tg.InputMessagesFilterPhoneCalls", got)
	}

	if calls.GetMissed() {
		t.Error("phone_calls filter must not set the Missed flag")
	}
}

func TestSearchFilterToTG_EmptyMeansNoFilter(t *testing.T) {
	got, err := searchFilterToTG("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := got.(*tg.InputMessagesFilterEmpty); !ok {
		t.Errorf("got %T, want *tg.InputMessagesFilterEmpty", got)
	}
}

func TestSearchFilterToTG_UnknownName(t *testing.T) {
	_, err := searchFilterToTG("bogus")
	if !errors.Is(err, ErrUnknownSearchFilter) {
		t.Errorf("got %v, want ErrUnknownSearchFilter", err)
	}
}

func TestIsSearchFilter(t *testing.T) {
	for name := range searchFilterCases() {
		if !IsSearchFilter(name) {
			t.Errorf("IsSearchFilter(%q) = false, want true", name)
		}
	}

	for _, name := range []string{"", "bogus", "PHOTOS"} {
		if IsSearchFilter(name) {
			t.Errorf("IsSearchFilter(%q) = true, want false", name)
		}
	}
}

func TestSearchFilters_SortedAndComplete(t *testing.T) {
	names := SearchFilters()

	if len(names) != len(searchFilterCases()) {
		t.Errorf("SearchFilters() has %d names, want %d", len(names), len(searchFilterCases()))
	}

	if !slices.IsSorted(names) {
		t.Errorf("SearchFilters() must be sorted, got %v", names)
	}
}

func TestIsSearchScope(t *testing.T) {
	for _, scope := range []string{SearchScopeUsers, SearchScopeGroups, SearchScopeChannels} {
		if !IsSearchScope(scope) {
			t.Errorf("IsSearchScope(%q) = false, want true", scope)
		}
	}

	for _, scope := range []string{"", "all", "broadcasts"} {
		if IsSearchScope(scope) {
			t.Errorf("IsSearchScope(%q) = true, want false", scope)
		}
	}
}
