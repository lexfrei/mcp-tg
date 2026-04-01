package main

import (
	"testing"

	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterTools(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-tg",
			Version: "test",
		},
		nil,
	)

	client := testutil.NoopClient{}
	registerTools(server, client, "/tmp/mcp-tg/downloads")
}
