// Package resources provides MCP resource handlers for Telegram data.
package resources

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultMessageLimit = 50
	mimeJSON            = "application/json"
	mimeText            = "text/plain"
)

// Register adds all Telegram resources and templates to the MCP server.
func Register(server *mcp.Server, client telegram.Client) {
	server.AddResource(dialogsResource(), dialogsHandler(client))
	server.AddResource(profileResource(), profileHandler(client))
	server.AddResourceTemplate(chatInfoTemplate(), chatInfoHandler(client))
	server.AddResourceTemplate(chatMessagesTemplate(), chatMessagesHandler(client))
}

func dialogsResource() *mcp.Resource {
	return &mcp.Resource{
		URI:         "tg://dialogs",
		Name:        "dialogs",
		Description: "List of all Telegram dialogs (chats, groups, channels)",
		MIMEType:    mimeJSON,
	}
}

func profileResource() *mcp.Resource {
	return &mcp.Resource{
		URI:         "tg://profile",
		Name:        "profile",
		Description: "Authenticated user's Telegram profile",
		MIMEType:    mimeJSON,
	}
}

func chatInfoTemplate() *mcp.ResourceTemplate {
	return &mcp.ResourceTemplate{
		URITemplate: "tg://chat/{peer}",
		Name:        "chat_info",
		Description: "Information about a Telegram chat, group, or channel",
		MIMEType:    mimeJSON,
	}
}

func chatMessagesTemplate() *mcp.ResourceTemplate {
	return &mcp.ResourceTemplate{
		URITemplate: "tg://chat/{peer}/messages",
		Name:        "chat_messages",
		Description: "Recent messages from a Telegram chat",
		MIMEType:    mimeText,
	}
}

func dialogsHandler(client telegram.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		dialogs, err := client.GetDialogs(ctx, telegram.DialogOpts{Limit: 100})
		if err != nil {
			return nil, errors.Wrap(err, "getting dialogs")
		}

		data, err := json.Marshal(dialogs)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling dialogs")
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: mimeJSON, Text: string(data)},
			},
		}, nil
	}
}

func profileHandler(client telegram.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		user, err := client.GetSelf(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "getting profile")
		}

		data, err := json.Marshal(user)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling profile")
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: mimeJSON, Text: string(data)},
			},
		}, nil
	}
}

func chatInfoHandler(client telegram.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		peerStr := extractPeer(req.Params.URI)
		if peerStr == "" {
			return nil, errors.New("missing peer in URI")
		}

		peer, err := client.ResolvePeer(ctx, peerStr)
		if err != nil {
			return nil, errors.Wrap(err, "resolving peer")
		}

		info, err := client.GetPeerInfo(ctx, peer)
		if err != nil {
			return nil, errors.Wrap(err, "getting peer info")
		}

		data, err := json.Marshal(info)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling peer info")
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: mimeJSON, Text: string(data)},
			},
		}, nil
	}
}

func chatMessagesHandler(client telegram.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		peerStr := extractPeer(req.Params.URI)
		if peerStr == "" {
			return nil, errors.New("missing peer in URI")
		}

		peer, err := client.ResolvePeer(ctx, peerStr)
		if err != nil {
			return nil, errors.Wrap(err, "resolving peer")
		}

		msgs, _, _, err := client.GetHistory(ctx, peer, telegram.HistoryOpts{Limit: defaultMessageLimit})
		if err != nil {
			return nil, errors.Wrap(err, "getting messages")
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: mimeText, Text: tools.FormatMessageList(msgs)},
			},
		}, nil
	}
}

const (
	chatURIPrefix  = "tg://chat/"
	messagesSuffix = "/messages"
)

func extractPeer(uri string) string {
	after, found := strings.CutPrefix(uri, chatURIPrefix)
	if !found {
		return ""
	}

	before, _ := strings.CutSuffix(after, messagesSuffix)

	return before
}

// ChatMessagesPeer extracts the peer identifier from a chat-messages resource
// URI (tg://chat/<peer>/messages). Unlike extractPeer it REQUIRES the /messages
// suffix, returning "" for a bare tg://chat/<peer> info URI or any non-chat URI.
// The subscribe/unsubscribe handlers use this to watch only the messages
// resource and ignore the chat-info resource, which has nothing to push.
func ChatMessagesPeer(uri string) string {
	after, found := strings.CutPrefix(uri, chatURIPrefix)
	if !found {
		return ""
	}

	peer, found := strings.CutSuffix(after, messagesSuffix)
	if !found {
		return ""
	}

	return peer
}
