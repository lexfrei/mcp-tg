package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultPublicSearchLimit = 20

// ErrPrivacyKeyRequired is returned when a privacy tool gets no key.
var ErrPrivacyKeyRequired = errors.New("key is required")

// ErrContactsRequired is returned when an import gets no contacts.
var ErrContactsRequired = errors.New("contacts is required")

// ErrFolderOrderRequired is returned when a reorder gets no folder IDs.
var ErrFolderOrderRequired = errors.New("folderIds is required")

func privacyKeyEnum() []any {
	keys := telegram.PrivacyKeys()

	values := make([]any, len(keys))
	for idx, key := range keys {
		values[idx] = key
	}

	return values
}

func privacyValueEnum() []any {
	return []any{telegram.PrivacyEverybody, telegram.PrivacyContacts, telegram.PrivacyCloseFriends, telegram.PrivacyNobody}
}

// PrivacyGetParams defines the parameters for the tg_privacy_get tool.
type PrivacyGetParams struct {
	Key string `json:"key" jsonschema:"Which setting: last_seen, phone_number, profile_photo, bio, forwards, group_invites, calls, ..."`
}

// PrivacyGetResult is the output of the tg_privacy_get tool.
type PrivacyGetResult struct {
	telegram.PrivacySetting

	Output string `json:"output"`
}

// NewPrivacyGetHandler creates a handler for the tg_privacy_get tool.
func NewPrivacyGetHandler(client telegram.Client) mcp.ToolHandlerFor[PrivacyGetParams, PrivacyGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params PrivacyGetParams,
	) (*mcp.CallToolResult, PrivacyGetResult, error) {
		setting, err := readPrivacy(ctx, client, params.Key)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PrivacyGetResult{}, err
		}

		return nil, PrivacyGetResult{PrivacySetting: *setting, Output: formatPrivacy(setting)}, nil
	}
}

func readPrivacy(ctx context.Context, client telegram.Client, key string) (*telegram.PrivacySetting, error) {
	if key == "" {
		return nil, validationErr(ErrPrivacyKeyRequired)
	}

	setting, err := client.GetPrivacy(ctx, key)
	if err != nil {
		return nil, telegramErr("failed to read privacy", err)
	}

	return setting, nil
}

// PrivacyGetTool returns the MCP tool definition for tg_privacy_get.
func PrivacyGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_privacy_get",
		Description: "Read one of the account's privacy settings: who may see it or do it, and the users excepted",
		InputSchema: inputSchemaWithEnum[PrivacyGetParams]("key", privacyKeyEnum()),
		Annotations: readOnlyAnnotations(),
	}
}

func formatPrivacy(setting *telegram.PrivacySetting) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%s: %s", setting.Key, setting.Value)

	if len(setting.AllowUsers) > 0 {
		fmt.Fprintf(&buf, ", always allowed: %v", setting.AllowUsers)
	}

	if len(setting.DisallowUsers) > 0 {
		fmt.Fprintf(&buf, ", never allowed: %v", setting.DisallowUsers)
	}

	return buf.String()
}

// PrivacySetParams defines the parameters for the tg_privacy_set tool.
type PrivacySetParams struct {
	Key           string   `json:"key"                     jsonschema:"Which setting, as in tg_privacy_get"`
	Value         string   `json:"value"                   jsonschema:"Who: everybody, contacts, close_friends or nobody"`
	AllowUsers    []string `json:"allowUsers,omitempty"    jsonschema:"Users always allowed, whatever the value"`
	DisallowUsers []string `json:"disallowUsers,omitempty" jsonschema:"Users never allowed, whatever the value"`
}

// PrivacySetResult is the output of the tg_privacy_set tool.
type PrivacySetResult struct {
	Output string `json:"output"`
}

// NewPrivacySetHandler creates a handler for the tg_privacy_set tool.
func NewPrivacySetHandler(client telegram.Client) mcp.ToolHandlerFor[PrivacySetParams, PrivacySetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params PrivacySetParams,
	) (*mcp.CallToolResult, PrivacySetResult, error) {
		update, err := privacyUpdate(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PrivacySetResult{}, err
		}

		err = client.SetPrivacy(ctx, update)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PrivacySetResult{}, telegramErr("failed to change privacy", err)
		}

		return nil, PrivacySetResult{Output: fmt.Sprintf("%s set to %s", params.Key, params.Value)}, nil
	}
}

func privacyUpdate(ctx context.Context, client telegram.Client, params *PrivacySetParams) (*telegram.PrivacyUpdate, error) {
	if params.Key == "" {
		return nil, validationErr(ErrPrivacyKeyRequired)
	}

	allow, err := resolvePeers(ctx, client, params.AllowUsers)
	if err != nil {
		return nil, err
	}

	disallow, err := resolvePeers(ctx, client, params.DisallowUsers)
	if err != nil {
		return nil, err
	}

	return &telegram.PrivacyUpdate{Key: params.Key, Value: params.Value, AllowUsers: allow, DisallowUsers: disallow}, nil
}

func resolvePeers(ctx context.Context, client telegram.Client, refs []string) ([]telegram.InputPeer, error) {
	peers := make([]telegram.InputPeer, 0, len(refs))

	for _, ref := range refs {
		peer, err := client.ResolvePeer(ctx, ref)
		if err != nil {
			return nil, telegramErr("failed to resolve "+ref, err)
		}

		peers = append(peers, peer)
	}

	return peers, nil
}

// PrivacySetTool returns the MCP tool definition for tg_privacy_set.
func PrivacySetTool() *mcp.Tool {
	schema := inputSchemaWithEnum[PrivacySetParams]("key", privacyKeyEnum())
	schema.Properties["value"].Enum = privacyValueEnum()

	return &mcp.Tool{
		Name:        "tg_privacy_set",
		Description: "Change one of the account's privacy settings, with optional always/never exceptions",
		InputSchema: schema,
		Annotations: idempotentAnnotations(),
	}
}

// ProfileDeletePhotoParams defines the parameters for the tg_profile_delete_photo tool.
type ProfileDeletePhotoParams struct{}

// ProfileDeletePhotoResult is the output of the tg_profile_delete_photo tool.
type ProfileDeletePhotoResult struct {
	Output string `json:"output"`
}

// NewProfileDeletePhotoHandler creates a handler for the tg_profile_delete_photo tool.
func NewProfileDeletePhotoHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ProfileDeletePhotoParams, ProfileDeletePhotoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ ProfileDeletePhotoParams,
	) (*mcp.CallToolResult, ProfileDeletePhotoResult, error) {
		err := client.DeleteProfilePhoto(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ProfileDeletePhotoResult{},
				telegramErr("failed to delete the profile photo", err)
		}

		return nil, ProfileDeletePhotoResult{Output: "Deleted the current profile photo"}, nil
	}
}

// ProfileDeletePhotoTool returns the MCP tool definition for tg_profile_delete_photo.
func ProfileDeletePhotoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_profile_delete_photo",
		Description: "Delete the account's current profile photo; the previous one, if any, becomes current",
		Annotations: destructiveAnnotations(),
	}
}

// ChatsSearchPublicParams defines the parameters for the tg_chats_search_public tool.
type ChatsSearchPublicParams struct {
	Query string `json:"query"           jsonschema:"Name or username to look for"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results (default 20)"`
}

// ChatsSearchPublicResult is the output of the tg_chats_search_public tool.
type ChatsSearchPublicResult struct {
	Count  int                 `json:"count"`
	Chats  []telegram.PeerInfo `json:"chats"`
	Output string              `json:"output"`
}

// NewChatsSearchPublicHandler creates a handler for the tg_chats_search_public tool.
func NewChatsSearchPublicHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ChatsSearchPublicParams, ChatsSearchPublicResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsSearchPublicParams,
	) (*mcp.CallToolResult, ChatsSearchPublicResult, error) {
		if strings.TrimSpace(params.Query) == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsSearchPublicResult{}, validationErr(ErrQueryRequired)
		}

		limit := params.Limit
		if limit <= 0 {
			limit = defaultPublicSearchLimit
		}

		chats, err := client.SearchPublic(ctx, params.Query, limit)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSearchPublicResult{}, telegramErr("public search failed", err)
		}

		return nil, ChatsSearchPublicResult{Count: len(chats), Chats: chats, Output: formatPeerInfos(chats)}, nil
	}
}

// ChatsSearchPublicTool returns the MCP tool definition for tg_chats_search_public.
func ChatsSearchPublicTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_chats_search_public",
		Description: "Search public Telegram channels, groups, bots and users by name or username — " +
			"beyond the chats the account already has",
		Annotations: readOnlyAnnotations(),
	}
}

func formatPeerInfos(infos []telegram.PeerInfo) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%d result(s)", len(infos))

	for idx := range infos {
		fmt.Fprintf(&buf, "\n%s (%s)", formatPeerRef(infos[idx].Title, infos[idx].Username, infos[idx].Peer), infos[idx].Type)
	}

	return buf.String()
}

// ContactsListParams defines the parameters for the tg_contacts_list tool.
type ContactsListParams struct{}

// ContactsListResult is the output of the tg_contacts_list tool.
type ContactsListResult struct {
	Count  int        `json:"count"`
	Users  []UserItem `json:"users"`
	Output string     `json:"output"`
}

// NewContactsListHandler creates a handler for the tg_contacts_list tool.
func NewContactsListHandler(client telegram.Client) mcp.ToolHandlerFor[ContactsListParams, ContactsListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ ContactsListParams,
	) (*mcp.CallToolResult, ContactsListResult, error) {
		users, err := client.ListContacts(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ContactsListResult{}, telegramErr("failed to list contacts", err)
		}

		return nil, ContactsListResult{Count: len(users), Users: usersToItems(users), Output: formatContacts(users)}, nil
	}
}

// ContactsListTool returns the MCP tool definition for tg_contacts_list.
func ContactsListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_list",
		Description: "List every contact of the Telegram account, with phone numbers where saved",
		Annotations: readOnlyAnnotations(),
	}
}

func formatContacts(users []telegram.User) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%d contact(s)", len(users))

	for idx := range users {
		buf.WriteString("\n" + formatUserName(&users[idx]))

		if users[idx].Phone != "" {
			buf.WriteString(" +" + strings.TrimPrefix(users[idx].Phone, "+"))
		}
	}

	return buf.String()
}

// ImportContact is one contact for tg_contacts_import.
type ImportContact struct {
	Phone     string `json:"phone"              jsonschema:"Phone number, international format"`
	FirstName string `json:"firstName"          jsonschema:"First name"`
	LastName  string `json:"lastName,omitempty" jsonschema:"Last name"`
}

// ContactsImportParams defines the parameters for the tg_contacts_import tool.
type ContactsImportParams struct {
	Contacts []ImportContact `json:"contacts" jsonschema:"Contacts to add by phone number"`
}

// ContactsImportResult is the output of the tg_contacts_import tool.
type ContactsImportResult struct {
	Imported int        `json:"imported"`
	Skipped  int        `json:"skipped"`
	Users    []UserItem `json:"users"`
	Output   string     `json:"output"`
}

// NewContactsImportHandler creates a handler for the tg_contacts_import tool.
func NewContactsImportHandler(client telegram.Client) mcp.ToolHandlerFor[ContactsImportParams, ContactsImportResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ContactsImportParams,
	) (*mcp.CallToolResult, ContactsImportResult, error) {
		if len(params.Contacts) == 0 {
			return &mcp.CallToolResult{IsError: true}, ContactsImportResult{}, validationErr(ErrContactsRequired)
		}

		contacts := make([]telegram.PhoneContact, len(params.Contacts))
		for idx, contact := range params.Contacts {
			contacts[idx] = telegram.PhoneContact(contact)
		}

		result, err := client.ImportContacts(ctx, contacts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ContactsImportResult{}, telegramErr("failed to import contacts", err)
		}

		return nil, ContactsImportResult{
			Imported: len(result.Users),
			Skipped:  result.Skipped,
			Users:    usersToItems(result.Users),
			Output: fmt.Sprintf("%d of %d number(s) belong to Telegram users and were added; %d skipped",
				len(result.Users), len(contacts), result.Skipped),
		}, nil
	}
}

// ContactsImportTool returns the MCP tool definition for tg_contacts_import.
func ContactsImportTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_import",
		Description: "Add contacts to the Telegram account by phone number; reports which numbers are Telegram users",
		Annotations: writeAnnotations(),
	}
}

// FoldersReorderParams defines the parameters for the tg_folders_reorder tool.
type FoldersReorderParams struct {
	FolderIDs []int `json:"folderIds" jsonschema:"Every folder ID, in the new order (from tg_folders_list)"`
}

// FoldersReorderResult is the output of the tg_folders_reorder tool.
type FoldersReorderResult struct {
	Output string `json:"output"`
}

// NewFoldersReorderHandler creates a handler for the tg_folders_reorder tool.
func NewFoldersReorderHandler(client telegram.Client) mcp.ToolHandlerFor[FoldersReorderParams, FoldersReorderResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params FoldersReorderParams,
	) (*mcp.CallToolResult, FoldersReorderResult, error) {
		if len(params.FolderIDs) == 0 {
			return &mcp.CallToolResult{IsError: true}, FoldersReorderResult{}, validationErr(ErrFolderOrderRequired)
		}

		err := client.ReorderFolders(ctx, params.FolderIDs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersReorderResult{}, telegramErr("failed to reorder folders", err)
		}

		return nil, FoldersReorderResult{Output: fmt.Sprintf("Folders reordered: %v", params.FolderIDs)}, nil
	}
}

// FoldersReorderTool returns the MCP tool definition for tg_folders_reorder.
func FoldersReorderTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_folders_reorder",
		Description: "Set the order of the account's chat folders",
		Annotations: idempotentAnnotations(),
	}
}
