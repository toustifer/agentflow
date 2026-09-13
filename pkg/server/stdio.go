package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FramingMode identifies the framing used for an MCP stdio message.
type FramingMode int

const (
	// FramingContentLength indicates standard Content-Length header framing.
	FramingContentLength FramingMode = iota
	// FramingNewline indicates newline-delimited JSON (NDJSON) framing.
	FramingNewline
)

// RPCRequest represents an incoming JSON-RPC request or notification.
type RPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	ID      any            `json:"id"`
}

// RPCResponse represents an outgoing JSON-RPC response.
type RPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
	ID      any       `json:"id"`
}

// RPCError represents a JSON-RPC error payload.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ReadMCPMessage reads one MCP frame using auto-detection between Content-Length
// header framing and Newline-delimited JSON framing.
// Leading whitespace (such as empty lines between frames) is skipped.
func ReadMCPMessage(reader *bufio.Reader) ([]byte, FramingMode, error) {
	for {
		peek, err := reader.Peek(1)
		if err != nil {
			return nil, FramingContentLength, err
		}
		b := peek[0]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			_, _ = reader.ReadByte()
			continue
		}
		break
	}

	// Sniff the first non-whitespace character.
	peek, err := reader.Peek(1)
	if err != nil {
		return nil, FramingContentLength, err
	}

	if peek[0] == 'C' || peek[0] == 'c' {
		return readContentLengthFrame(reader)
	}
	return readNewlineFrame(reader)
}

func readContentLengthFrame(reader *bufio.Reader) ([]byte, FramingMode, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, FramingContentLength, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			return nil, FramingContentLength, fmt.Errorf("invalid MCP header line %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(key), "content-length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, FramingContentLength, fmt.Errorf("invalid Content-Length header %q", line)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, FramingContentLength, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, FramingContentLength, err
	}
	return body, FramingContentLength, nil
}

func readNewlineFrame(reader *bufio.Reader) ([]byte, FramingMode, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) && len(line) == 0 {
			return nil, FramingNewline, io.EOF
		}
		if !errors.Is(err, io.EOF) {
			return nil, FramingNewline, err
		}
	}
	line = strings.TrimRight(line, "\r\n")
	return []byte(line), FramingNewline, nil
}

// WriteMCPMessage writes one MCP frame to out matching the given framing mode.
// If mode is FramingContentLength, it writes Content-Length headers followed by payload.
// If mode is FramingNewline, it writes payload directly followed by a single newline.
func WriteMCPMessage(out io.Writer, payload []byte, mode FramingMode) error {
	if mode == FramingContentLength {
		if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
			return err
		}
		if _, err := out.Write(payload); err != nil {
			return err
		}
	} else {
		if _, err := out.Write(payload); err != nil {
			return err
		}
		if _, err := out.Write([]byte("\n")); err != nil {
			return err
		}
	}
	if flusher, ok := out.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}
	return nil
}

// ServeMCP runs the MCP server loop over in and out with auto-detected adaptive framing.
func (s *Server) ServeMCP(ctx context.Context, in io.Reader, out io.Writer) error {
	return ServeMCP(ctx, in, out, s)
}

// ServeMCP runs the MCP server loop over in and out with auto-detected adaptive framing.
func ServeMCP(ctx context.Context, in io.Reader, out io.Writer, srv *Server) error {
	reader := bufio.NewReader(in)

	for {
		body, mode, err := ReadMCPMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var req RPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}

		if req.ID == nil {
			// MCP notifications carry no ID and expect no response.
			continue
		}

		resp := RPCResponse{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "agentflow", "version": "0.1.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": srv.Tools()}
		case "tools/call":
			name, _ := req.Params["name"].(string)
			args, _ := req.Params["arguments"].(map[string]any)
			data, callErr := srv.Handle(ctx, name, args)
			if callErr != nil {
				resp.Error = &RPCError{Code: -32603, Message: callErr.Error()}
			} else {
				resp.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": FormatToolResult(data)}}}
			}
		default:
			// Direct method dispatch for file-mode bridge.
			data, callErr := srv.Handle(ctx, req.Method, req.Params)
			if callErr != nil {
				resp.Error = &RPCError{Code: -32603, Message: callErr.Error()}
			} else {
				resp.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": FormatToolResult(data)}}}
			}
		}

		payload, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if err := WriteMCPMessage(out, payload, mode); err != nil {
			return err
		}
	}
}

// FormatToolResult serializes a tool result value to string.
func FormatToolResult(v any) string {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(payload)
}
