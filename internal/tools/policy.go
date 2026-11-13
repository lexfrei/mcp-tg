package tools

import (
	"maps"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// accountWideTools act on every chat of the account rather than on a peer
// the caller names, so TELEGRAM_ALLOWED_PEERS cannot scope them. They are
// hidden whenever the allowlist is set; telegram.PeerAllowlist refuses the
// client calls behind them as well, in case one is reached another way.
// A new tool that reads or changes chats without taking a peer belongs here.
func accountWideTools() []*mcp.Tool {
	return []*mcp.Tool{
		MessagesSearchGlobalTool(),
		MessagesClearAllDraftsTool(),
		FoldersListTool(),
		FoldersCreateTool(),
		FoldersEditTool(),
		FoldersDeleteTool(),
	}
}

func isAccountWide(name string) bool {
	return slices.ContainsFunc(accountWideTools(), func(tool *mcp.Tool) bool { return tool.Name == name })
}

// ToolPolicy decides which tools the server offers. The zero value offers
// every tool. A tool the policy rejects is never registered, so a client
// can neither list nor call it.
type ToolPolicy struct {
	readOnly   bool
	peerScoped bool
	only       map[string]struct{}
	disabled   map[string]struct{}
}

// NewToolPolicy builds the policy from the access settings: readOnly keeps
// only read-only tools, only (when non-empty) keeps only the named tools,
// disabled drops the named tools, and peerScoped drops the account-wide
// tools an allowlist cannot scope.
func NewToolPolicy(readOnly bool, only, disabled []string, peerScoped bool) ToolPolicy {
	return ToolPolicy{
		readOnly:   readOnly,
		peerScoped: peerScoped,
		only:       nameSet(only),
		disabled:   nameSet(disabled),
	}
}

// Admits reports whether the server should offer tool. The build-version
// tool is always offered: it never touches Telegram, and it is how an
// operator checks which binary is answering.
func (policy *ToolPolicy) Admits(tool *mcp.Tool) bool {
	if tool.Name == ServerVersionToolName {
		return true
	}

	if policy.only != nil {
		if _, ok := policy.only[tool.Name]; !ok {
			return false
		}
	}

	if _, ok := policy.disabled[tool.Name]; ok {
		return false
	}

	if policy.peerScoped && isAccountWide(tool.Name) {
		return false
	}

	return !policy.readOnly || isReadOnly(tool)
}

// named returns every tool name the policy mentions, sorted.
func (policy *ToolPolicy) named() []string {
	names := slices.Collect(maps.Keys(policy.only))
	names = slices.AppendSeq(names, maps.Keys(policy.disabled))
	slices.Sort(names)

	return slices.Compact(names)
}

func isReadOnly(tool *mcp.Tool) bool {
	return tool.Annotations != nil && tool.Annotations.ReadOnlyHint
}

func nameSet(names []string) map[string]struct{} {
	if len(names) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}

	return set
}

// Registry is what AddTool records into and consults: the bool fields the
// coercion middleware needs, the policy that decides whether a tool is
// registered at all, and the name of every tool offered to it — admitted or
// not — so a policy naming a tool that does not exist can be caught.
type Registry struct {
	BoolFields BoolFieldRegistry
	policy     ToolPolicy
	offered    map[string]struct{}
}

// NewRegistry returns an empty registry that registers what policy admits.
func NewRegistry(policy ToolPolicy) *Registry {
	return &Registry{
		BoolFields: BoolFieldRegistry{},
		policy:     policy,
		offered:    map[string]struct{}{},
	}
}

// UnknownTools returns the tool names the policy mentions that were never
// offered to AddTool. A typo in TELEGRAM_DISABLED_TOOLS would otherwise
// leave the tool it meant to disable quietly enabled.
func (registry *Registry) UnknownTools() []string {
	var unknown []string

	for _, name := range registry.policy.named() {
		if _, ok := registry.offered[name]; !ok {
			unknown = append(unknown, name)
		}
	}

	return unknown
}

// admit records the tool as offered and reports whether to register it.
func (registry *Registry) admit(tool *mcp.Tool) bool {
	registry.offered[tool.Name] = struct{}{}

	return registry.policy.Admits(tool)
}
