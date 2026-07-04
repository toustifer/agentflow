package server

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

func TestReadResponseParsesWrongJSONRPCValue(t *testing.T) {
	body := `{"jsonrpc":"1.0","id":1,"result":{}}`
	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	bridge := &BTBridge{
		stdout: bufio.NewReader(strings.NewReader(frame)),
	}
	resp, err := bridge.readResponseLocked()
	if err != nil {
		t.Fatal(err)
	}
	if resp.JSONRPC != "1.0" {
		t.Fatalf("expected jsonrpc 1.0, got %#v", resp)
	}
}

func TestReadResponseRejectsOversizedFrame(t *testing.T) {
	bridge := &BTBridge{
		stdout: bufio.NewReader(strings.NewReader("Content-Length: 99999999\r\n\r\n")),
	}
	_, err := bridge.readResponseLocked()
	if err == nil {
		t.Fatal("expected oversized frame error")
	}
}

func TestBridgeRPCRejectsNotStarted(t *testing.T) {
	bridge := NewBTBridge()
	_, err := bridge.RPC("ping", map[string]any{})
	if err == nil {
		t.Fatal("expected bt_service not started error")
	}
}
