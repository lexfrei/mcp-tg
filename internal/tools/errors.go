// Package tools provides MCP tool handlers for Telegram operations.
package tools

import "github.com/cockroachdb/errors"

// ErrValidation indicates invalid parameters provided by the caller.
var ErrValidation = errors.New("validation error")

// ErrTelegram indicates a failure communicating with the Telegram API.
var ErrTelegram = errors.New("telegram request error")

// ErrPeerRequired is returned when a peer parameter is missing.
var ErrPeerRequired = errors.New("peer is required")

// ErrMessageIDRequired is returned when a message ID parameter is missing.
var ErrMessageIDRequired = errors.New("message ID is required")

// ErrTextRequired is returned when a text parameter is missing.
var ErrTextRequired = errors.New("text is required")

// ErrQueryRequired is returned when a search query parameter is missing.
var ErrQueryRequired = errors.New("query is required")

// ErrNegativeLimit is returned when a numeric limit parameter is negative.
var ErrNegativeLimit = errors.New("numeric limits must not be negative")

// ErrTitleRequired is returned when a title parameter is missing.
var ErrTitleRequired = errors.New("title is required")

// ErrGroupRequired is returned when a group parameter is missing.
var ErrGroupRequired = errors.New("group is required")

// ErrUserRequired is returned when a user parameter is missing.
var ErrUserRequired = errors.New("user is required")

// ErrLinkRequired is returned when a link parameter is missing.
var ErrLinkRequired = errors.New("link is required")

// ErrPathRequired is returned when a file path parameter is missing.
var ErrPathRequired = errors.New("path is required")

// ErrPathsRequired is returned when a paths list parameter is missing.
var ErrPathsRequired = errors.New("paths list is required")

// ErrFirstNameRequired is returned when a first name parameter is missing.
var ErrFirstNameRequired = errors.New("first name is required")

// ErrMessageNotFound is returned when a message cannot be found.
var ErrMessageNotFound = errors.New("message not found")

// ErrNameRequired is returned when a name parameter is missing.
var ErrNameRequired = errors.New("name is required")

// ErrFolderIDRequired is returned when a folder ID parameter is missing.
var ErrFolderIDRequired = errors.New("folder ID is required")

// ErrStickerFileIDRequired is returned when a sticker file ID parameter is missing.
var ErrStickerFileIDRequired = errors.New("sticker file ID is required")

// validationErr marks an error as a validation error.
func validationErr(err error) error {
	//nolint:wrapcheck // Mark adds a sentinel category, the caller already provides context.
	return errors.Mark(err, ErrValidation)
}

// telegramErr wraps a message and underlying error as a Telegram request error.
func telegramErr(msg string, err error) error {
	//nolint:wrapcheck // Mark adds a sentinel category on top of Wrap which provides context.
	return errors.Mark(errors.Wrap(err, msg), ErrTelegram)
}
