// Package resources provides MCP resource handlers for Telegram data.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultMessageLimit = 50

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
		MIMEType:    "application/json",
	}
}

func profileResource() *mcp.Resource {
	return &mcp.Resource{
		URI:         "tg://profile",
		Name:        "profile",
		Description: "Authenticated user's Telegram profile",
		MIMEType:    "application/json",
	}
}

func chatInfoTemplate() *mcp.ResourceTemplate {
	return &mcp.ResourceTemplate{
		URITemplate: "tg://chat/{peer}",
		Name:        "chat_info",
		Description: "Information about a Telegram chat, group, or channel",
		MIMEType:    "application/json",
	}
}

func chatMessagesTemplate() *mcp.ResourceTemplate {
	return &mcp.ResourceTemplate{
		URITemplate: "tg://chat/{peer}/messages",
		Name:        "chat_messages",
		Description: "Recent messages from a Telegram chat",
		MIMEType:    "application/json",
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
				{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
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
				{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
			},
		}, nil
	}
}

func chatInfoHandler(client telegram.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		peerStr := extractPeer(req.Params.URI, "tg://chat/", "/messages")
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
				{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
			},
		}, nil
	}
}

func chatMessagesHandler(client telegram.Client) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		peerStr := extractPeer(req.Params.URI, "tg://chat/", "/messages")
		if peerStr == "" {
			return nil, errors.New("missing peer in URI")
		}

		peer, err := client.ResolvePeer(ctx, peerStr)
		if err != nil {
			return nil, errors.Wrap(err, "resolving peer")
		}

		msgs, _, err := client.GetHistory(ctx, peer, telegram.HistoryOpts{Limit: defaultMessageLimit})
		if err != nil {
			return nil, errors.Wrap(err, "getting messages")
		}

		var buf strings.Builder

		for idx := range msgs {
			fmt.Fprintf(&buf, "[%d] %s\n", msgs[idx].ID, msgs[idx].Text)
		}

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "text/plain", Text: buf.String()},
			},
		}, nil
	}
}

func extractPeer(uri, prefix, suffix string) string {
	after, found := strings.CutPrefix(uri, prefix)
	if !found {
		return ""
	}

	before, _ := strings.CutSuffix(after, suffix)

	return before
}
