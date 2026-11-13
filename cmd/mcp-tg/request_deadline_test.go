package main

import (
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestRequestDeadline_ReadsAndSendsGetTheDeadline(t *testing.T) {
	t.Parallel()

	for _, input := range []bin.Encoder{
		&tg.MessagesGetHistoryRequest{},
		&tg.MessagesSendMessageRequest{},
		&tg.ContactsResolveUsernameRequest{},
	} {
		if got := requestDeadline(input); got != requestTimeout {
			t.Errorf("%s: deadline %s, want %s", requestName(input), got, requestTimeout)
		}
	}
}

// gotd resends a request whose deadline passed, so anything the server-error
// and connection-loss middlewares refuse to resend must not get one either.
func TestRequestDeadline_HeldBackRequestsWaitWithoutOne(t *testing.T) {
	t.Parallel()

	for _, input := range []bin.Encoder{
		&tg.ChannelsCreateChannelRequest{},
		&tg.MessagesCreateChatRequest{},
		&tg.MessagesUpdateDialogFilterRequest{},
		&tg.PhotosUploadProfilePhotoRequest{},
		&tg.MessagesExportChatInviteRequest{},
		&tg.MessagesEditExportedChatInviteRequest{},
	} {
		if safeToResend(input) {
			t.Fatalf("%s: expected safeToResend to hold it back", requestName(input))
		}

		if got := requestDeadline(input); got != 0 {
			t.Errorf("%s: deadline %s, want none", requestName(input), got)
		}
	}
}

func TestRequestDeadline_FileTransferWaitsWithoutOne(t *testing.T) {
	t.Parallel()

	for _, input := range []bin.Encoder{
		&tg.UploadSaveFilePartRequest{},
		&tg.UploadSaveBigFilePartRequest{},
		&tg.UploadGetFileRequest{},
	} {
		if got := requestDeadline(input); got != 0 {
			t.Errorf("%s: deadline %s, want none", requestName(input), got)
		}
	}
}

// A login step the server ran but did not answer must not be sent again: a
// second sendCode or resendCode issues a new code and voids the first, and a
// second signIn or checkPassword spends a code or SRP challenge already used.
func TestRequestDeadline_LoginStepsWaitWithoutOne(t *testing.T) {
	t.Parallel()

	for _, input := range []bin.Encoder{
		&tg.AuthSendCodeRequest{},
		&tg.AuthResendCodeRequest{},
		&tg.AuthSignInRequest{},
		&tg.AuthCheckPasswordRequest{},
	} {
		if got := requestDeadline(input); got != 0 {
			t.Errorf("%s: deadline %s, want none", requestName(input), got)
		}
	}
}
