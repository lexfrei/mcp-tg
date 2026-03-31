package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- tg_profile_set_name ---

// ProfileSetNameParams defines the parameters for the tg_profile_set_name tool.
type ProfileSetNameParams struct {
	FirstName string `json:"firstName" jsonschema:"New first name"`
	LastName  string `json:"lastName"  jsonschema:"New last name"`
}

// ProfileSetNameResult is the output of the tg_profile_set_name tool.
type ProfileSetNameResult struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Output    string `json:"output"`
}

// NewProfileSetNameHandler creates a handler for the tg_profile_set_name tool.
func NewProfileSetNameHandler(client telegram.Client) mcp.ToolHandlerFor[ProfileSetNameParams, ProfileSetNameResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ProfileSetNameParams,
	) (*mcp.CallToolResult, ProfileSetNameResult, error) {
		if params.FirstName == "" {
			return &mcp.CallToolResult{IsError: true}, ProfileSetNameResult{},
				validationErr(ErrFirstNameRequired)
		}

		err := client.SetProfileName(ctx, params.FirstName, params.LastName)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ProfileSetNameResult{},
				telegramErr("failed to set profile name", err)
		}

		return nil, ProfileSetNameResult{
			FirstName: params.FirstName,
			LastName:  params.LastName,
			Output:    "Updated profile name to " + params.FirstName + " " + params.LastName,
		}, nil
	}
}

// ProfileSetNameTool returns the MCP tool definition for tg_profile_set_name.
func ProfileSetNameTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_profile_set_name",
		Description: "Set the authenticated user's first and last name",
		Annotations: idempotentAnnotations(),
	}
}

// --- tg_profile_set_bio ---

// ProfileSetBioParams defines the parameters for the tg_profile_set_bio tool.
type ProfileSetBioParams struct {
	Bio string `json:"bio" jsonschema:"New bio text (max 70 characters)"`
}

// ProfileSetBioResult is the output of the tg_profile_set_bio tool.
type ProfileSetBioResult struct {
	Bio    string `json:"bio"`
	Output string `json:"output"`
}

// NewProfileSetBioHandler creates a handler for the tg_profile_set_bio tool.
func NewProfileSetBioHandler(client telegram.Client) mcp.ToolHandlerFor[ProfileSetBioParams, ProfileSetBioResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ProfileSetBioParams,
	) (*mcp.CallToolResult, ProfileSetBioResult, error) {
		err := client.SetProfileBio(ctx, params.Bio)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ProfileSetBioResult{},
				telegramErr("failed to set profile bio", err)
		}

		return nil, ProfileSetBioResult{
			Bio:    params.Bio,
			Output: "Updated profile bio",
		}, nil
	}
}

// ProfileSetBioTool returns the MCP tool definition for tg_profile_set_bio.
func ProfileSetBioTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_profile_set_bio",
		Description: "Set the authenticated user's bio text",
		Annotations: idempotentAnnotations(),
	}
}

// --- tg_profile_set_photo ---

// ProfileSetPhotoParams defines the parameters for the tg_profile_set_photo tool.
type ProfileSetPhotoParams struct {
	Path string `json:"path" jsonschema:"Local file path of the new profile photo"`
}

// ProfileSetPhotoResult is the output of the tg_profile_set_photo tool.
type ProfileSetPhotoResult struct {
	Output string `json:"output"`
}

// NewProfileSetPhotoHandler creates a handler for the tg_profile_set_photo tool.
func NewProfileSetPhotoHandler(client telegram.Client) mcp.ToolHandlerFor[ProfileSetPhotoParams, ProfileSetPhotoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ProfileSetPhotoParams,
	) (*mcp.CallToolResult, ProfileSetPhotoResult, error) {
		if params.Path == "" {
			return &mcp.CallToolResult{IsError: true}, ProfileSetPhotoResult{},
				validationErr(ErrPathRequired)
		}

		err := client.SetProfilePhoto(ctx, params.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ProfileSetPhotoResult{},
				telegramErr("failed to set profile photo", err)
		}

		return nil, ProfileSetPhotoResult{
			Output: "Updated profile photo",
		}, nil
	}
}

// ProfileSetPhotoTool returns the MCP tool definition for tg_profile_set_photo.
func ProfileSetPhotoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_profile_set_photo",
		Description: "Set the authenticated user's profile photo",
		Annotations: idempotentAnnotations(),
	}
}
