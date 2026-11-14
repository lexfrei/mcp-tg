package telegram

import (
	"errors"
	"testing"

	"github.com/gotd/td/tg"
)

func TestFreeFolderID_TakesTheSmallestUnusedFromTwo(t *testing.T) {
	filters := []tg.DialogFilterClass{
		&tg.DialogFilterDefault{},
		&tg.DialogFilter{ID: 2},
		&tg.DialogFilterChatlist{ID: 3},
		&tg.DialogFilter{ID: 72},
	}

	got, err := freeFolderID(filters)
	if err != nil || got != 4 {
		t.Errorf("freeFolderID = %d, %v; want 4 (2 and 3 are taken, chatlists included)", got, err)
	}
}

func TestFreeFolderID_NeverHandsOutAReservedID(t *testing.T) {
	got, err := freeFolderID(nil)
	if err != nil || got != firstFolderID {
		t.Errorf("freeFolderID on an empty account = %d, %v; want %d (0 and 1 are reserved)", got, err, firstFolderID)
	}
}

func TestFreeFolderID_FullAccount(t *testing.T) {
	filters := make([]tg.DialogFilterClass, 0, lastFolderID)
	for folderID := firstFolderID; folderID <= lastFolderID; folderID++ {
		filters = append(filters, &tg.DialogFilter{ID: folderID})
	}

	_, err := freeFolderID(filters)
	if !errors.Is(err, ErrNoFreeFolderID) {
		t.Errorf("err = %v, want ErrNoFreeFolderID", err)
	}
}
