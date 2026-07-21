package business

import (
	"testing"

	"github.com/pragun/brain/internal/flavor"
)

func mockServer() flavor.MCPServer {
	return flavor.MCPServer{Name: "mock", Command: "python3", Args: []string{"testdata/mockserver.py"}}
}

func TestMCPHandshakeAndToolsList(t *testing.T) {
	c, err := Connect(mockServer())
	if err != nil {
		t.Fatalf("connect/handshake: %v", err)
	}
	defer c.Close()

	tools, err := c.Tools()
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "sales" {
		t.Fatalf("tools = %+v, want one 'sales'", tools)
	}
}

func TestMCPToolCallReturnsText(t *testing.T) {
	c, err := Connect(mockServer())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	out, err := c.Call("sales", nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out == "" || out[:3] != "wk1" {
		t.Errorf("unexpected tool output: %q", out)
	}
}

func TestMCPToolErrorIsReported(t *testing.T) {
	c, _ := Connect(mockServer())
	defer c.Close()
	if _, err := c.Call("nonexistent", nil); err == nil {
		t.Error("a tool reporting isError must surface as an error")
	}
}

func TestGatherCollectsFromServers(t *testing.T) {
	srcs, err := Gather([]flavor.MCPServer{mockServer()},
		[]ToolCall{{Server: "mock", Tool: "sales"}})
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(srcs) != 1 || srcs[0].Tool != "sales" {
		t.Fatalf("sources = %+v", srcs)
	}
}
