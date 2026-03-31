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
