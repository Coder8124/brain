// Package business is the assistant that reaches outward.
//
// Where tutor and secretary work entirely from local data, business mode talks
// to MCP servers — the same protocol Claude and other hosts speak — to pull in
// spreadsheets, dashboards, and databases, then summarises the trends in them.
package business

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/pragun/brain/internal/flavor"
)

// Client is a minimal MCP client over stdio. It speaks just enough of the
// protocol to be useful — initialize, list tools, call tools — rather than the
// whole surface; business mode reads data, it does not need prompts, resources
// subscriptions, or sampling.
type Client struct {
	name string
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	mu   sync.Mutex
	id   int
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message) }

// Tool is one capability a server exposes.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Connect launches a server and completes the MCP handshake. The caller must
// Close it.
func Connect(server flavor.MCPServer) (*Client, error) {
	cmd := exec.Command(server.Command, server.Args...)
	if len(server.Env) > 0 {
		cmd.Env = envSlice(server.Env)
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// A server's stderr is its logs; let it flow to ours rather than blocking
	// the pipe when it fills.
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching %s: %w", server.Name, err)
	}

	c := &Client{name: server.Name, cmd: cmd, in: in, out: bufio.NewReader(out)}

	// initialize handshake: request, then the required initialized notification.
	if _, err := c.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "brain", "version": "0.1.0"},
	}); err != nil {
		c.Close()
		return nil, err
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Close() error {
	if c.in != nil {
		c.in.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		// Give it a moment to exit on stdin close, then make sure it is gone.
		done := make(chan struct{})
		go func() { c.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			c.cmd.Process.Kill()
		}
	}
	return nil
}

// Tools lists what the server can do.
func (c *Client) Tools() ([]Tool, error) {
	raw, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var res struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// Call invokes a tool and returns its text content flattened to a string, which
// is what the summariser wants — business mode reads data, it does not render
// rich tool results.
func (c *Client) Call(tool string, args map[string]any) (string, error) {
	raw, err := c.call("tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return "", err
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	var text string
	for _, part := range res.Content {
		if part.Type == "text" {
			text += part.Text + "\n"
		}
	}
	if res.IsError {
		return text, fmt.Errorf("tool %s reported an error", tool)
	}
	return text, nil
}

// call sends a request and blocks for its response. MCP over stdio is
// newline-delimited JSON; we serialise access so interleaved calls cannot mix
// their reads.
func (c *Client) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.id++
	req := rpcRequest{JSONRPC: "2.0", ID: c.id, Method: method, Params: params}
	if err := c.write(req); err != nil {
		return nil, err
	}

	// Skip any notifications the server emits (they have no id) until our
	// matching response arrives.
	for {
		line, err := c.out.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("%s: reading response: %w", c.name, err)
		}
		var resp rpcResponse
		if json.Unmarshal(line, &resp) != nil || resp.ID != c.id {
			continue
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = c.in.Write(b)
	return err
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
