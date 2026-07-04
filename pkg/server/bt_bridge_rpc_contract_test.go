package server

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"
)

type nopWriteCloser struct {
	bytes.Buffer
}

func (n *nopWriteCloser) Close() error { return nil }

func TestBridgeRPCRejectsResponseIDMismatch(t *testing.T) {
	stdin := &nopWriteCloser{}
	body := `{"jsonrpc":"2.0","id":999,"result":{"ok":true}}`
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	bridge := &BTBridge{
		stdin:   stdin,
		stdout:  bufio.NewReader(bytes.NewBufferString(frame)),
		started: true,
	}

	_, err := bridge.RPC("ping", map[string]any{})
	if err == nil {
		t.Fatal("expected response id mismatch error")
	}
	if got := err.Error(); got == "" || !contains(got, "response id mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBridgeRPCRejectsInvalidJSONRPCVersion(t *testing.T) {
	stdin := &nopWriteCloser{}
	body := `{"jsonrpc":"1.0","id":1,"result":{"ok":true}}`
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	bridge := &BTBridge{
		stdin:   stdin,
		stdout:  bufio.NewReader(bytes.NewBufferString(frame)),
		started: true,
	}

	_, err := bridge.RPC("ping", map[string]any{})
	if err == nil {
		t.Fatal("expected invalid jsonrpc version error")
	}
	if got := err.Error(); got == "" || !contains(got, "invalid jsonrpc version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || bytes.Contains([]byte(s), []byte(sub)))
}
