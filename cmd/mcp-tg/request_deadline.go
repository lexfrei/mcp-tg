package main

import (
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// requestTimeout bounds how long a query may sit on the primary connection
// without an answer. Since gotd v0.164 a query the server never answers waits
// for the caller's context, and an MCP tool call carries no deadline, so a
// half-dead connection would hold the call forever.
const requestTimeout = 30 * time.Second

// requestDeadline is telegram.Options.RequestTimeout. When the deadline passes
// gotd reconnects and sends the query once more, so a query safeToResend holds
// back gets no deadline: the resend is exactly the duplicate it exists to
// prevent. File parts get none either: a part can honestly take longer than
// requestTimeout on a slow link, and a resend would restart it. Nor do login
// steps: a resent sendCode issues a new code and voids the one the user is
// reading, and a resent signIn or checkPassword spends a code already used.
func requestDeadline(input bin.Encoder) time.Duration {
	if !safeToResend(input) {
		return 0
	}

	switch input.(type) {
	case *tg.UploadSaveFilePartRequest,
		*tg.UploadSaveBigFilePartRequest,
		*tg.UploadGetFileRequest,
		*tg.AuthSendCodeRequest,
		*tg.AuthResendCodeRequest,
		*tg.AuthSignInRequest,
		*tg.AuthCheckPasswordRequest:
		return 0
	default:
		return requestTimeout
	}
}
