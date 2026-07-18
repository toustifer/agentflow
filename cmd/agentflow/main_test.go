package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
	lwserver "github.com/toustifer/agentflow/pkg/server"
)

func TestToolsListSerializesInputSchemaAsObjectForEveryTool(t *testing.T) {
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, eng.Close()) })

	srv, err := lwserver.New(eng, lwserver.Config{})
	require.NoError(t, err)

	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	input := bytes.NewBufferString(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(request), request))
	var output bytes.Buffer
	require.NoError(t, serveMCP(context.Background(), input, &output, srv))

	_, payload, ok := bytes.Cut(output.Bytes(), []byte("\r\n\r\n"))
	require.True(t, ok, "MCP response must include a header/body separator")

	var response struct {
		Result struct {
			Tools []map[string]json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(payload, &response))
	require.NotEmpty(t, response.Result.Tools)

	for _, tool := range response.Result.Tools {
		var name string
		require.NoError(t, json.Unmarshal(tool["name"], &name))

		schema, exists := tool["inputSchema"]
		require.Truef(t, exists, "tool %q must include inputSchema", name)

		var object map[string]json.RawMessage
		require.NoErrorf(t, json.Unmarshal(schema, &object), "tool %q inputSchema must be a JSON object", name)
		require.NotNilf(t, object, "tool %q inputSchema must not be null", name)
	}
}
