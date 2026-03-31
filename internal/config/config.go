// Package config provides configuration loading from environment variables.
package config

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cockroachdb/errors"
)

const (
	maxPort    = 65535
	defaultDir = ".mcp-tg"
)

// ErrAppIDRequired is returned when TELEGRAM_APP_ID is not set.
var ErrAppIDRequired = errors.New("TELEGRAM_APP_ID is required")

// ErrAppHashRequired is returned when TELEGRAM_APP_HASH is not set.
var ErrAppHashRequired = errors.New("TELEGRAM_APP_HASH is required")

// ErrInvalidAppID is returned when TELEGRAM_APP_ID is not a valid positive integer.
var ErrInvalidAppID = errors.New("TELEGRAM_APP_ID must be a positive integer")

// ErrInvalidHTTPPort is returned when MCP_HTTP_PORT is not a valid port number.
var ErrInvalidHTTPPort = errors.New("MCP_HTTP_PORT must be a valid port number (1-65535)")

// Config holds the application configuration loaded from environment variables.
type Config struct {
	AppID       int
	AppHash     string
	Phone       string
	Password    string
	SessionFile string
	AuthCode    string
	DownloadDir string
	HTTPPort    string
	HTTPHost    string
}

// Load reads configuration from environment variables and returns a Config.
func Load() (*Config, error) {
	appIDStr := os.Getenv("TELEGRAM_APP_ID")
	if appIDStr == "" {
		return nil, ErrAppIDRequired
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil || appID < 1 {
		return nil, ErrInvalidAppID
	}

	appHash := os.Getenv("TELEGRAM_APP_HASH")
	if appHash == "" {
		return nil, ErrAppHashRequired
	}

	sessionFile := os.Getenv("TELEGRAM_SESSION_FILE")
	if sessionFile == "" {
		home, _ := os.UserHomeDir()
		sessionFile = filepath.Join(home, defaultDir, "session.json")
	}

	downloadDir := os.Getenv("TELEGRAM_DOWNLOAD_DIR")
	if downloadDir == "" {
		downloadDir = filepath.Join(os.TempDir(), "mcp-tg", "downloads")
	}

	httpPort := os.Getenv("MCP_HTTP_PORT")
	if httpPort != "" {
		port, portErr := strconv.Atoi(httpPort)
		if portErr != nil || port < 1 || port > maxPort {
			return nil, ErrInvalidHTTPPort
		}
	}

	httpHost := os.Getenv("MCP_HTTP_HOST")
	if httpHost == "" {
		httpHost = "127.0.0.1"
	}

	return &Config{
		AppID:       appID,
		AppHash:     appHash,
		Phone:       os.Getenv("TELEGRAM_PHONE"),
		Password:    os.Getenv("TELEGRAM_PASSWORD"),
		SessionFile: sessionFile,
		AuthCode:    os.Getenv("TELEGRAM_AUTH_CODE"),
		DownloadDir: downloadDir,
		HTTPPort:    httpPort,
		HTTPHost:    httpHost,
	}, nil
}

// HasPassword returns true if a 2FA password is configured.
func (c *Config) HasPassword() bool {
	return c.Password != ""
}

// HTTPEnabled returns true if HTTP transport should be enabled.
func (c *Config) HTTPEnabled() bool {
	return c.HTTPPort != ""
}

// HTTPAddr returns the full host:port address for the HTTP server.
func (c *Config) HTTPAddr() string {
	return net.JoinHostPort(c.HTTPHost, c.HTTPPort)
}
