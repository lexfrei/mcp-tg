// Package completions provides MCP completion handlers for argument autocompletion.
package completions

import (
	"context"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxCompletions = 20

// Argument names that take a peer reference.
const (
	argPeer   = "peer"
	argSendAs = "sendAs"
)

// completesPeer reports whether an argument takes a peer reference and so
// can be completed from the dialog list. sendAs takes one too, though the
// dialogs it offers are wider than the identities the chat would accept —
// only channels.getSendAs knows those, and it needs the sibling peer
// argument, which the completion request does not carry.
func completesPeer(argument string) bool {
	return argument == argPeer || argument == argSendAs
}

// NewHandler returns a CompletionHandler that provides peer autocompletion
// by searching the user's dialogs.
func NewHandler(client telegram.Client) func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
		if !completesPeer(req.Params.Argument.Name) {
			return &mcp.CompleteResult{}, nil
		}

		prefix := req.Params.Argument.Value
		if prefix == "" {
			return &mcp.CompleteResult{}, nil
		}

		dialogs, err := client.GetDialogs(ctx, telegram.DialogOpts{Limit: 100})
		if err != nil {
			return &mcp.CompleteResult{}, nil //nolint:nilerr // gracefully degrade on error.
		}

		values := matchDialogs(dialogs, prefix)

		return &mcp.CompleteResult{
			Completion: mcp.CompletionResultDetails{
				Values:  values,
				HasMore: len(values) >= maxCompletions,
			},
		}, nil
	}
}

func matchDialogs(dialogs []telegram.Dialog, prefix string) []string {
	lower := strings.ToLower(prefix)
	values := make([]string, 0, maxCompletions)

	for _, dlg := range dialogs {
		if len(values) >= maxCompletions {
			break
		}

		if matchesPrefix(dlg.Username, lower) {
			values = append(values, "@"+dlg.Username)
		} else if matchesPrefix(dlg.Title, lower) {
			values = append(values, dlg.Title)
		}
	}

	return values
}

func matchesPrefix(val, lowerPrefix string) bool {
	return val != "" && strings.HasPrefix(strings.ToLower(val), lowerPrefix)
}
