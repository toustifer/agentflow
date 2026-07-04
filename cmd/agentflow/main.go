package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
	lwserver "github.com/toustifer/agentflow/pkg/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "stdio" {
		if err := runMCPStdio(); err != nil {
			log.Fatalf("mcp server exited: %v", err)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "file" {
		if err := runFile(os.Args[2]); err != nil {
			log.Fatalf("file: %v", err)
		}
		return
	}
	if err := runHTTP(); err != nil {
		log.Fatalf("agentflow startup failed: %v", err)
	}
}

func runFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	components, err := buildComponents()
	if err != nil {
		return err
	}
	defer components.close()

	var req rpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	out, callErr := components.server.Handle(context.Background(), req.Method, req.Params)
	if callErr != nil {
		resp.Error = &rpcError{Code: -32603, Message: callErr.Error()}
	} else {
		resp.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": formatToolResult(out)}}}
	}
	payload, _ := json.Marshal(resp)
	fmt.Print(string(payload))
	return nil
}

func runHTTP() error {
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║  agentflow - Temporal lightweight engine ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	components, err := buildComponents()
	if err != nil {
		return err
	}
	defer components.close()

	fmt.Println("[1/3] Lightweight engine and server assembled")
	fmt.Printf("  ✓ Engine ready: %T\n", components.engine)
	fmt.Printf("  ✓ Server ready: %T\n", components.server)
	fmt.Printf("  ✓ Server tools (%d): %s\n", len(components.tools), strings.Join(toolNames(components.tools), ", "))
	fmt.Printf("  ✓ Persistence probe: %s\n", components.backendName)

	fmt.Println("\n[2/3] Starting HTTP health endpoint...")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"store":  "sqlite-in-memory",
			"backend": components.backendName,
		})
	})

	mux.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"namespaces": []string{},
			"note":       "agentflow v0.1 - Temporal persistence engine",
		})
	})

	httpServer := &http.Server{Addr: "127.0.0.1:9600", Handler: mux}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		fmt.Println("\nShutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	fmt.Println("\n[3/3] agentflow ready")
	fmt.Println("  ┌───────────────────────────────────┐")
	fmt.Println("  │  HTTP :9600 (health + future API) │")
	fmt.Println("  │  MCP    (assembled, not served)   │")
	fmt.Println("  └───────────────────────────────────┘")
	fmt.Println()
	fmt.Printf("Backend: %s\n", components.backendName)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runMCPStdio() error {
	components, err := buildComponents()
	if err != nil {
		return err
	}
	defer components.close()

	return serveMCP(context.Background(), os.Stdin, os.Stdout, components.server)
}

type runtimeComponents struct {
	engine      *engine.Engine
	server      *lwserver.Server
	tools       []lwserver.ToolSpec
	backendName string
}

func buildComponents() (*runtimeComponents, error) {
	dbPath := os.Getenv("AGENTFLOW_DB_PATH")
	if dbPath == "" {
		tmpDir := os.Getenv("TMPDIR")
		if tmpDir == "" {
			tmpDir = os.Getenv("TEMP")
		}
		if tmpDir == "" {
			tmpDir = os.TempDir()
		}
		dbPath = filepath.Join(tmpDir, "agentflow.db")
	}
	backendName := "sqlite-file"
	if dbPath == ":memory:" {
		backendName = "sqlite-in-memory"
	}
	cfg := engine.NewEngineConfig{DBPath: dbPath}

	eng, err := engine.NewEngine(cfg)
	if err != nil {
		return nil, err
	}

	srv, err := lwserver.New(eng, lwserver.Config{})
	if err != nil {
		_ = eng.Close()
		return nil, err
	}

	return &runtimeComponents{
		engine:      eng,
		server:      srv,
		tools:       srv.Tools(),
		backendName: backendName,
	}, nil
}

func (c *runtimeComponents) close() {
	if c == nil {
		return
	}
	if err := c.engine.Close(); err != nil {
		log.Printf("engine shutdown error: %v", err)
	}
}

func toolNames(tools []lwserver.ToolSpec) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
	ID      any            `json:"id"`
}

type rpcResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	Result  any            `json:"result,omitempty"`
	Error   *rpcError      `json:"error,omitempty"`
	ID      any            `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// serveMCP runs the MCP server loop.
// initialize, tools/list, and tools/call follow the MCP protocol.
// All other methods (direct tool names like "namespace_create" etc.)
// are dispatched through Server.Handle for the file-mode bridge.
func serveMCP(ctx context.Context, in io.Reader, out io.Writer, srv *lwserver.Server) error {
	reader := bufio.NewReader(in)
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for {
		req, err := decodeRPC(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
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
				resp.Error = &rpcError{Code: -32603, Message: callErr.Error()}
			} else {
				resp.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": formatToolResult(data)}}}
			}
		default:
			// Direct method dispatch for file-mode bridge.
			data, callErr := srv.Handle(ctx, req.Method, req.Params)
			if callErr != nil {
				resp.Error = &rpcError{Code: -32603, Message: callErr.Error()}
			} else {
				resp.Result = map[string]any{"content": []any{map[string]any{"type": "text", "text": formatToolResult(data)}}}
			}
		}

		if err := encodeRPC(writer, resp); err != nil {
			return err
		}
	}
}

func decodeRPC(r *bufio.Reader) (*rpcRequest, error) {
	for {
		peek, err := r.Peek(1)
		if err != nil {
			return nil, err
		}
		if peek[0] != '\r' && peek[0] != '\n' {
			break
		}
		r.Discard(1)
	}

	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSuffix(line, "\r\n")
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, errors.New("missing or empty Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func encodeRPC(w *bufio.Writer, resp rpcResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := w.WriteString(header); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}

func formatToolResult(v any) string {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(payload)
}
