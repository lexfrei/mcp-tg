package telegram

import (
	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// Folder IDs are chosen by the client. 0 is the "All chats" list and 1
// the archive, so a folder takes the smallest unused ID from 2, which is
// what the official clients do; they stop at 255.
const (
	firstFolderID = 2
	lastFolderID  = 255
)

// ErrNoFreeFolderID marks an account whose folder IDs are all taken.
var ErrNoFreeFolderID = errors.New("no free folder id")

// freeFolderID returns the smallest folder ID not used by filters.
// messages.updateDialogFilter without one is refused with FILTER_ID_INVALID.
func freeFolderID(filters []tg.DialogFilterClass) (int, error) {
	used := make(map[int]bool, len(filters))

	for _, raw := range filters {
		switch filter := raw.(type) {
		case *tg.DialogFilter:
			used[filter.ID] = true
		case *tg.DialogFilterChatlist:
			used[filter.ID] = true
		}
	}

	for folderID := firstFolderID; folderID <= lastFolderID; folderID++ {
		if !used[folderID] {
			return folderID, nil
		}
	}

	return 0, ErrNoFreeFolderID
}
