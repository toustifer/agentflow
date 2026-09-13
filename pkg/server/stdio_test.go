package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// framedRequest wraps body in standard Content-Length framing.
func framedRequest(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

// parseContentLengthFrame extracts the JSON body of a Content-Length framed message.
func parseContentLengthFrame(t *testing.T, s string) []byte {
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

func TestServeMCPContentLengthFraming(t *testing.T) {
	srv := newTestServer(t)

	input := bytes.NewBufferString(framedRequest(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	var output bytes.Buffer
	require.NoError(t, srv.ServeMCP(context.Background(), input, &output))

	require.Contains(t, output.String(), "Content-Length:")
	body := parseContentLengthFrame(t, output.String())

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

func TestServeMCPNewlineDelimitedJSONFraming(t *testing.T) {
	srv := newTestServer(t)

	input := bytes.NewBufferString("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}\n")
	var output bytes.Buffer
	require.NoError(t, srv.ServeMCP(context.Background(), input, &output))

	outStr := output.String()
	require.NotContains(t, outStr, "Content-Length:")
	require.True(t, strings.HasSuffix(outStr, "\n"), "newline-delimited output must end with newline")

	trimmed := strings.TrimRight(outStr, "\r\n")
	require.NotContains(t, trimmed, "\n", "newline-delimited output must be a single line JSON")

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
	require.NoError(t, json.Unmarshal([]byte(trimmed), &response))
	require.Equal(t, "2.0", response.JSONRPC)
	require.Equal(t, 1, response.ID)
	require.Equal(t, "2024-11-05", response.Result.ProtocolVersion)
	require.Equal(t, "agentflow", response.Result.ServerInfo.Name)
}

func TestServeMCPAdaptiveFramingMixed(t *testing.T) {
	srv := newTestServer(t)

	// Send 3 requests in a single stream with mixed framing:
	// 1: Newline JSON tools/list
	// 2: Content-Length tools/list
	// 3: Newline JSON initialize
	req1 := "{\"jsonrpc\":\"2.0\",\"id\":101,\"method\":\"tools/list\",\"params\":{}}\n"
	req2 := framedRequest(`{"jsonrpc":"2.0","id":102,"method":"tools/list","params":{}}`)
	req3 := "{\"jsonrpc\":\"2.0\",\"id\":103,\"method\":\"initialize\",\"params\":{}}\n"

	input := bytes.NewBufferString(req1 + req2 + req3)
	var output bytes.Buffer
	require.NoError(t, srv.ServeMCP(context.Background(), input, &output))

	outStr := output.String()

	// Response 1 should be newline JSON:
	idx1 := strings.Index(outStr, "\n")
	require.Positive(t, idx1)
	line1 := outStr[:idx1]
	require.NotContains(t, line1, "Content-Length:")
	var resp1 struct {
		ID     int `json:"id"`
		Result struct {
			Tools []any `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(line1), &resp1))
	require.Equal(t, 101, resp1.ID)
	require.NotEmpty(t, resp1.Result.Tools)

	// Remainder starts with response 2 (Content-Length framed)
	rem := outStr[idx1+1:]
	require.True(t, strings.HasPrefix(rem, "Content-Length:"))
	body2 := parseContentLengthFrame(t, rem)
	var resp2 struct {
		ID     int `json:"id"`
		Result struct {
			Tools []any `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body2, &resp2))
	require.Equal(t, 102, resp2.ID)

	// After response 2 body comes response 3 (newline JSON)
	hEnd := strings.Index(rem, "\r\n\r\n")
	rem3 := rem[hEnd+4+len(body2):]
	rem3Trimmed := strings.TrimSpace(rem3)
	require.NotContains(t, rem3Trimmed, "Content-Length:")
	var resp3 struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(rem3Trimmed), &resp3))
	require.Equal(t, 103, resp3.ID)
}

func TestServeMCPIgnoresNotificationsInBothModes(t *testing.T) {
	srv := newTestServer(t)

	// Send notification in newline format, then request in newline format
	input := bytes.NewBufferString(
		"{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n" +
			"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"initialize\",\"params\":{}}\n",
	)
	var output bytes.Buffer
	require.NoError(t, srv.ServeMCP(context.Background(), input, &output))

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 1, "notification should not produce a response")
	var resp struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &resp))
	require.Equal(t, 2, resp.ID)
}

func TestReadMCPMessageAutoDetect(t *testing.T) {
	// 1. Standard Content-Length
	clPayload := `{"hello":"world"}`
	clInput := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(clPayload), clPayload)
	body, mode, err := ReadMCPMessage(bufio.NewReader(strings.NewReader(clInput)))
	require.NoError(t, err)
	require.Equal(t, FramingContentLength, mode)
	require.Equal(t, clPayload, string(body))

	// 2. Content-Length with leading whitespace and lower-case header
	clInput2 := fmt.Sprintf("\r\n\n  content-length: %d\r\n\r\n%s", len(clPayload), clPayload)
	body, mode, err = ReadMCPMessage(bufio.NewReader(strings.NewReader(clInput2)))
	require.NoError(t, err)
	require.Equal(t, FramingContentLength, mode)
	require.Equal(t, clPayload, string(body))

	// 3. Newline-delimited JSON
	ndInput := "{\"jsonrpc\":\"2.0\",\"id\":1}\n"
	body, mode, err = ReadMCPMessage(bufio.NewReader(strings.NewReader(ndInput)))
	require.NoError(t, err)
	require.Equal(t, FramingNewline, mode)
	require.Equal(t, `{"jsonrpc":"2.0","id":1}`, string(body))

	// 4. Newline-delimited JSON with leading empty lines
	ndInput2 := "\r\n\n{\"jsonrpc\":\"2.0\",\"id\":2}\r\n"
	body, mode, err = ReadMCPMessage(bufio.NewReader(strings.NewReader(ndInput2)))
	require.NoError(t, err)
	require.Equal(t, FramingNewline, mode)
	require.Equal(t, `{"jsonrpc":"2.0","id":2}`, string(body))

	// 5. Newline-delimited JSON at EOF without trailing newline
	ndInput3 := "{\"jsonrpc\":\"2.0\",\"id\":3}"
	body, mode, err = ReadMCPMessage(bufio.NewReader(strings.NewReader(ndInput3)))
	require.NoError(t, err)
	require.Equal(t, FramingNewline, mode)
	require.Equal(t, `{"jsonrpc":"2.0","id":3}`, string(body))

	// 6. Clean EOF
	_, _, err = ReadMCPMessage(bufio.NewReader(strings.NewReader("   \r\n  \n")))
	require.ErrorIs(t, err, io.EOF)
}

func TestWriteMCPMessage(t *testing.T) {
	payload := []byte(`{"result":"ok"}`)

	var bufCL bytes.Buffer
	require.NoError(t, WriteMCPMessage(&bufCL, payload, FramingContentLength))
	require.Equal(t, fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload), bufCL.String())

	var bufND bytes.Buffer
	require.NoError(t, WriteMCPMessage(&bufND, payload, FramingNewline))
	require.Equal(t, string(payload)+"\n", bufND.String())
}

