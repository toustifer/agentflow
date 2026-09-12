package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
	lwserver "github.com/toustifer/agentflow/pkg/server"
)

// framedRequest wraps body in the standard MCP stdio Content-Length framing
// (the framing @modelcontextprotocol/sdk clients use).
func framedRequest(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

// parseFrame extracts the JSON body of the first Content-Length framed message
// in s and asserts the framing is well-formed.
func parseFrame(t *testing.T, s string) []byte {
	t.Helper()

	headerEnd := strings.Index(s, "\r\n\r\n")
	require.Positive(t, headerEnd, "output must contain a Content-Length header terminator")

	contentLength := -1
	for _, line := range strings.Split(s[:headerEnd], "\r\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "content-length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			require.NoError(t, err)
			contentLength = n
		}
	}
	require.Greater(t, contentLength, 0, "output must carry a positive Content-Length header")

	body := s[headerEnd+4 : headerEnd+4+contentLength]
	require.Len(t, body, contentLength)
	return []byte(body)
}

func newTestServer(t *testing.T) *lwserver.Server {
	t.Helper()

	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eng.Close()) })

	srv, err := lwserver.New(eng, lwserver.Config{})
	require.NoError(t, err)
	return srv
}

// TestServeMCPContentLengthFraming pins the standard MCP stdio framing: the
// server must accept Content-Length framed requests and answer with the same
// framing. The old newline-delimited implementation crashed on "Content-Length"
// headers with "invalid character 'C' looking for beginning of value".
func TestServeMCPContentLengthFraming(t *testing.T) {
	srv := newTestServer(t)

	input := bytes.NewBufferString(framedRequest(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	var output bytes.Buffer
	require.NoError(t, serveMCP(context.Background(), input, &output, srv))

	require.Contains(t, output.String(), "Content-Length:")
	body := parseFrame(t, output.String())

	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, "2.0", response.JSONRPC)
	require.Equal(t, 1, response.ID)
	require.Equal(t, "2024-11-05", response.Result.ProtocolVersion)
	require.Equal(t, "agentflow", response.Result.ServerInfo.Name)
}

// TestServeMCPToolsListFramed covers the tools/list payload over the framed
// protocol: every tool must carry a non-null object inputSchema and
// project_inspect must expose namespace_id in properties and required.
func TestServeMCPToolsListFramed(t *testing.T) {
	srv := newTestServer(t)

	input := bytes.NewBufferString(framedRequest(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	var output bytes.Buffer
	require.NoError(t, serveMCP(context.Background(), input, &output, srv))

	var response struct {
		Result struct {
			Tools []map[string]json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(parseFrame(t, output.String()), &response))
	require.NotEmpty(t, response.Result.Tools)

	var projectInspectSchema map[string]json.RawMessage
	for _, tool := range response.Result.Tools {
		var name string
		require.NoError(t, json.Unmarshal(tool["name"], &name))

		schema, exists := tool["inputSchema"]
		require.Truef(t, exists, "tool %q must include inputSchema", name)

		var object map[string]json.RawMessage
		require.NoErrorf(t, json.Unmarshal(schema, &object), "tool %q inputSchema must be a JSON object", name)
		require.NotNilf(t, object, "tool %q inputSchema must not be null", name)
		if name == "project_inspect" {
			projectInspectSchema = object
		}
	}

	require.Contains(t, string(projectInspectSchema["properties"]), `"namespace_id"`)
	require.Contains(t, string(projectInspectSchema["required"]), `"namespace_id"`)
}

// TestServeMCPIgnoresNotifications verifies a request without an id (an MCP
// notification) is consumed without producing any response.
func TestServeMCPIgnoresNotifications(t *testing.T) {
	srv := newTestServer(t)

	input := bytes.NewBufferString(
		framedRequest(`{"jsonrpc":"2.0","method":"notifications/initialized"}`) +
			framedRequest(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`),
	)
	var output bytes.Buffer
	require.NoError(t, serveMCP(context.Background(), input, &output, srv))

	// Exactly one framed response: the notification must not be answered.
	require.Equal(t, 1, strings.Count(output.String(), "Content-Length:"))
	body := parseFrame(t, output.String())
	var response struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, 2, response.ID)
}

// TestServeMCPToleratesTrailingWhitespace pins the lenient frame boundary: a
// stray newline after a complete frame (what naive piped input appends) must
// not be misread as an empty header block, and the next frame must still
// parse.
func TestServeMCPToleratesTrailingWhitespace(t *testing.T) {
	srv := newTestServer(t)

	input := bytes.NewBufferString(
		framedRequest(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) + "\n" +
			framedRequest(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`),
	)
	var output bytes.Buffer
	require.NoError(t, serveMCP(context.Background(), input, &output, srv))

	require.Equal(t, 2, strings.Count(output.String(), "Content-Length:"))
}
