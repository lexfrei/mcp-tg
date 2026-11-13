package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrInvalidReadOnly is returned when TELEGRAM_READ_ONLY is not a valid boolean.
var ErrInvalidReadOnly = errors.New("TELEGRAM_READ_ONLY must be a boolean (1/0/true/false)")

// ErrAccessListEmpty is returned when one of the access lists
// (TELEGRAM_ALLOWED_PEERS, TELEGRAM_TOOLS, TELEGRAM_DISABLED_TOOLS) is set but
// parses to no entries. The operator asked for a boundary; reading a value
// like " , " as "no list" would fail open, the same reasoning as
// ErrFileRootsEmpty.
var ErrAccessListEmpty = errors.New("access list is set but contains no entries")

// Access holds the operator's limits on what the server may do with the
// account. The zero value imposes none.
type Access struct {
	// AllowedPeers, when non-empty, is the only set of peers any tool,
	// resource or prompt may address. Entries are peer identifiers in any
	// form ResolvePeer accepts.
	AllowedPeers []string

	// ReadOnly registers only the tools annotated read-only.
	ReadOnly bool

	// Tools, when non-empty, registers only the named tools.
	Tools []string

	// DisabledTools never registers the named tools.
	DisabledTools []string
}

// loadAccess reads the access-control variables.
func loadAccess() (Access, error) {
	readOnly, err := loadReadOnly()
	if err != nil {
		return Access{}, err
	}

	peers, err := parseList("TELEGRAM_ALLOWED_PEERS", os.Getenv("TELEGRAM_ALLOWED_PEERS"))
	if err != nil {
		return Access{}, err
	}

	enabled, err := parseList("TELEGRAM_TOOLS", os.Getenv("TELEGRAM_TOOLS"))
	if err != nil {
		return Access{}, err
	}

	disabled, err := parseList("TELEGRAM_DISABLED_TOOLS", os.Getenv("TELEGRAM_DISABLED_TOOLS"))
	if err != nil {
		return Access{}, err
	}

	return Access{
		AllowedPeers:  peers,
		ReadOnly:      readOnly,
		Tools:         enabled,
		DisabledTools: disabled,
	}, nil
}

func loadReadOnly() (bool, error) {
	raw := os.Getenv("TELEGRAM_READ_ONLY")
	if raw == "" {
		return false, nil
	}

	readOnly, err := strconv.ParseBool(raw)
	if err != nil {
		return false, ErrInvalidReadOnly
	}

	return readOnly, nil
}

// parseList splits a comma-separated variable, trimming blanks around each
// entry and dropping empty ones. An empty value means nil; a value with only
// separators and blanks is ErrAccessListEmpty. The variable is read by the
// caller, by literal name, so the docs pin on the environment table can see it.
func parseList(name, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}

	var entries []string

	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		return nil, errors.Wrap(ErrAccessListEmpty, name)
	}

	return entries, nil
}
