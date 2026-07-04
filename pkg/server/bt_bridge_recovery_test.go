package server

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

type failingWriteCloser struct {
	err error
}

func (f failingWriteCloser) Write(p []byte) (int, error) { return 0, f.err }
func (f failingWriteCloser) Close() error                { return nil }

func TestBridgeRPCBrokenPipeOnHeaderWrite(t *testing.T) {
	bridge := &BTBridge{
		stdin:   failingWriteCloser{err: io.ErrClosedPipe},
		stdout:  bufio.NewReader(bytes.NewBuffer(nil)),
		started: true,
	}

	_, err := bridge.RPC("ping", map[string]any{})
	if err == nil {
		t.Fatal("expected broken pipe error")
	}
	if got := err.Error(); got == "" || !contains(got, "write header") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBridgeReadResponseEarlyExit(t *testing.T) {
	bridge := &BTBridge{
		stdout: bufio.NewReader(bytes.NewBuffer(nil)),
	}
	_, err := bridge.readResponseLocked()
	if err == nil {
		t.Fatal("expected read header error on early exit")
	}
	if got := err.Error(); got == "" || !contains(got, "read header") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBridgeStopLockedResetsStateWithoutProcess(t *testing.T) {
	bridge := &BTBridge{started: true, stdin: &nopWriteCloser{}}
	bridge.stopLocked()
	if bridge.started {
		t.Fatal("expected started=false after stopLocked")
	}
	if bridge.stdin != nil || bridge.stdout != nil || bridge.cmd != nil {
		t.Fatalf("expected fields cleared, got %#v", bridge)
	}
}

func TestBridgeRPCReturnsNotStartedAfterStop(t *testing.T) {
	bridge := NewBTBridge()
	bridge.started = true
	bridge.stopLocked()
	_, err := bridge.RPC("ping", map[string]any{})
	if err == nil {
		t.Fatal("expected not started error")
	}
	if !errors.Is(err, err) && err.Error() == "" {
		t.Fatalf("unexpected error: %v", err)
	}
}
