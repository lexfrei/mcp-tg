package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/lexfrei/mcp-tg/internal/config"
)

func TestNewResolver_NoProxyKeepsTheDefault(t *testing.T) {
	resolver, err := newResolver(nil)
	if err != nil || resolver != nil {
		t.Fatalf("resolver = %v, err = %v; want nil for gotd's default", resolver, err)
	}
}

func TestNewResolver_BuildsBothKinds(t *testing.T) {
	for _, settings := range []*config.Proxy{
		{Kind: config.ProxySOCKS5, Addr: "127.0.0.1:1080", User: "alice", Password: "s3cret"},
		{Kind: config.ProxyMTProxy, Addr: "proxy.example.org:443", Secret: make([]byte, 16)},
	} {
		resolver, err := newResolver(settings)
		if err != nil || resolver == nil {
			t.Errorf("%s: resolver = %v, err = %v", settings.Kind, resolver, err)
		}
	}
}

// The SOCKS5 dialer must actually go to the proxy: a server that accepts the
// connection proves the dial was routed there rather than to the target.
func TestSOCKS5Dialer_ConnectsToTheProxy(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	reached := make(chan struct{})

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			close(reached)
			conn.Close()
		}
	}()

	dial, err := socks5Dialer(&config.Proxy{Kind: config.ProxySOCKS5, Addr: listener.Addr().String()})
	if err != nil {
		t.Fatalf("socks5Dialer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// The handshake fails against a server that closes at once; reaching it
	// is what this test is about.
	_, _ = dial(ctx, "tcp", "149.154.167.50:443")

	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("the dial never reached the proxy")
	}
}
