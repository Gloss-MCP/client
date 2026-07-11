package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gloss-mcp/client/internal/plugins"
	"github.com/gloss-mcp/client/internal/store"
)

func TestListenAndServeServesThenShutsDownOnCancel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/a.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		Root:          root,
		RepoName:      "fixture",
		ConnectorType: store.ConnectorLocal,
		Port:          0,
		Registry:      plugins.NewRegistry(plugins.NewPlaintext()),
	})

	ctx, cancel := context.WithCancel(context.Background())
	addrCh := make(chan string, 1)
	doneCh := make(chan error, 1)

	go func() {
		doneCh <- srv.ListenAndServe(ctx, func(addr string) { addrCh <- addr })
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(5 * time.Second):
		t.Fatal("server never became ready")
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET http://%s/: %v", addr, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("ListenAndServe returned %v, want nil after graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}

func TestListenAndServePortInUse(t *testing.T) {
	root := t.TempDir()

	cfg := Config{
		Root:          root,
		RepoName:      "fixture",
		ConnectorType: store.ConnectorLocal,
		Port:          0,
		Registry:      plugins.NewRegistry(plugins.NewPlaintext()),
	}

	first := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addrCh := make(chan string, 1)
	go func() { _ = first.ListenAndServe(ctx, func(addr string) { addrCh <- addr }) }()

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(5 * time.Second):
		t.Fatal("first server never became ready")
	}

	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("strconv.Atoi(%q): %v", portStr, err)
	}

	second := New(Config{
		Root: root, RepoName: "fixture", ConnectorType: store.ConnectorLocal,
		Port: port, Registry: plugins.NewRegistry(plugins.NewPlaintext()),
	})
	if err := second.ListenAndServe(context.Background(), nil); err == nil {
		t.Error("second ListenAndServe on the same port succeeded, want an error")
	}
}
