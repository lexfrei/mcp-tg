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
	appID, err := loadAppID()
	if err != nil {
		return nil, err
	}

	appHash, err := loadAppHash()
	if err != nil {
		return nil, err
	}

	httpPort, err := loadHTTPPort()
	if err != nil {
		return nil, err
	}

	return &Config{
		AppID:       appID,
		AppHash:     appHash,
		Phone:       os.Getenv("TELEGRAM_PHONE"),
		Password:    os.Getenv("TELEGRAM_PASSWORD"),
		SessionFile: loadSessionFile(),
		AuthCode:    os.Getenv("TELEGRAM_AUTH_CODE"),
		DownloadDir: loadDownloadDir(),
		HTTPPort:    httpPort,
		HTTPHost:    loadHTTPHost(),
	}, nil
}

// HasPassword returns true if a 2FA password is configured.
func (cfg *Config) HasPassword() bool {
	return cfg.Password != ""
}

// HTTPEnabled returns true if HTTP transport should be enabled.
func (cfg *Config) HTTPEnabled() bool {
	return cfg.HTTPPort != ""
}

// HTTPAddr returns the full host:port address for the HTTP server.
func (cfg *Config) HTTPAddr() string {
	return net.JoinHostPort(cfg.HTTPHost, cfg.HTTPPort)
}

func loadAppID() (int, error) {
	raw := os.Getenv("TELEGRAM_APP_ID")
	if raw == "" {
		return 0, ErrAppIDRequired
	}

	appID, err := strconv.Atoi(raw)
	if err != nil || appID < 1 {
		return 0, ErrInvalidAppID
	}

	return appID, nil
}

func loadAppHash() (string, error) {
	appHash := os.Getenv("TELEGRAM_APP_HASH")
	if appHash == "" {
		return "", ErrAppHashRequired
	}

	return appHash, nil
}

func loadSessionFile() string {
	sessionFile := os.Getenv("TELEGRAM_SESSION_FILE")
	if sessionFile != "" {
		return sessionFile
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), defaultDir, "session.json")
	}

	return filepath.Join(home, defaultDir, "session.json")
}

func loadDownloadDir() string {
	dir := os.Getenv("TELEGRAM_DOWNLOAD_DIR")
	if dir != "" {
		return dir
	}

	return filepath.Join(os.TempDir(), "mcp-tg", "downloads")
}

func loadHTTPPort() (string, error) {
	httpPort := os.Getenv("MCP_HTTP_PORT")
	if httpPort == "" {
		return "", nil
	}

	port, err := strconv.Atoi(httpPort)
	if err != nil || port < 1 || port > maxPort {
		return "", ErrInvalidHTTPPort
	}

	return httpPort, nil
}

func loadHTTPHost() string {
	host := os.Getenv("MCP_HTTP_HOST")
	if host != "" {
		return host
	}

	return "127.0.0.1"
}
