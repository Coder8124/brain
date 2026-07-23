// Package mcpserver exposes brain's persistent memory as an MCP server.
//
// This is the "memory is the product" thesis made real: any MCP host — Claude
// Desktop, Claude Code, Cursor — connects to this over stdio and gains one
// local, private memory that follows the user across every tool and every
// session. The memory lives in the user's own vault; nothing is uploaded. The
// same store the brain app reads is the store an external agent writes to, so
// what you tell one, the others know.
//
// It speaks MCP over newline-delimited JSON-RPC 2.0 on stdio — the transport
// every MCP host supports — mirroring the client in internal/business.
package mcpserver

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pragun/brain/internal/memory"
	"github.com/pragun/brain/internal/provider"
	"github.com/pragun/brain/internal/router"
)

const protocolVersion = "2024-11-05"

// Server holds the memory store and the embedding backend it recalls against.
// embed may be nil — the store then works without vectors (recall falls back to
// salience order), which is what keeps the transport testable without a live
// model.
type Server struct {
	DB         *sql.DB
	embed      *provider.Provider
	embedModel string
	out        *json.Encoder
}

func New(db *sql.DB, rt *router.Router) *Server {
	embed, _ := rt.Model(router.T0)
	return &Server{DB: db, embed: rt.Local(), embedModel: embed}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Serve runs the request loop until stdin closes. Read errors end the loop;
// per-request errors become JSON-RPC error responses so the host stays healthy.
func (s *Server) Serve(in io.Reader, w io.Writer) error {
	if err := memory.Init(s.DB); err != nil {
		return err
	}
	s.out = json.NewEncoder(w)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue // unparseable line: ignore, do not crash the transport
		}
		s.handle(req)
	}
	return sc.Err()
}

func (s *Server) handle(req request) {
	switch req.Method {
	case "initialize":
		s.reply(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "brain-memory", "version": "0.1.0"},
		})
	case "notifications/initialized":
		// notification, no reply
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": toolDefs})
	case "tools/call":
		s.callTool(req)
	default:
		if len(req.ID) > 0 {
			s.replyErr(req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

func (s *Server) callTool(req request) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.replyErr(req.ID, -32602, "bad params")
		return
	}
	var args map[string]any
	json.Unmarshal(p.Arguments, &args)

	text, err := s.dispatch(p.Name, args)
	if err != nil {
		// MCP convention: tool errors are results with isError, not protocol
		// errors, so the model sees them and can react.
		s.reply(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		})
		return
	}
	s.reply(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	})
}

func (s *Server) dispatch(name string, args map[string]any) (string, error) {
	switch name {
	case "remember":
		return s.remember(argStr(args, "text"), argStr(args, "kind"))
	case "recall":
		return s.recall(argStr(args, "query"), argInt(args, "limit", 5))
	case "list_memories":
		return s.listMemories()
	case "forget":
		return s.forget(argStr(args, "id"))
	}
	return "", fmt.Errorf("unknown tool %q", name)
}

// --- memory operations ---

func (s *Server) remember(text, kindStr string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("remember needs text")
	}
	kind := memory.Fact
	switch memory.Kind(kindStr) {
	case memory.Preference:
		kind = memory.Preference
	case memory.Person:
		kind = memory.Person
	case memory.Context:
		kind = memory.Context
	}
	added, err := memory.Store(s.DB, s.embed, s.embedModel, &memory.Memory{
		Text: text, Kind: kind, Salience: 0.7, Source: "mcp",
	})
	if err != nil {
		return "", err
	}
	if !added {
		return "Already knew that (reinforced the existing memory).", nil
	}
	return "Remembered.", nil
}

func (s *Server) recall(query string, k int) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("recall needs a query")
	}
	mems, err := memory.Recall(s.DB, s.embed, s.embedModel, query, k)
	if err != nil {
		return "", err
	}
	if len(mems) == 0 {
		return "No relevant memories.", nil
	}
	var b strings.Builder
	for _, m := range mems {
		fmt.Fprintf(&b, "- (%s) %s\n", m.Kind, m.Text)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *Server) listMemories() (string, error) {
	mems, err := memory.All(s.DB)
	if err != nil {
		return "", err
	}
	if len(mems) == 0 {
		return "No memories yet.", nil
	}
	var b strings.Builder
	for _, m := range mems {
		fmt.Fprintf(&b, "[%d] (%s) %s\n", m.ID, m.Kind, m.Text)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (s *Server) forget(idStr string) (string, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil {
		return "", fmt.Errorf("forget needs a numeric memory id")
	}
	if err := memory.Forget(s.DB, id); err != nil {
		return "", err
	}
	return "Forgotten.", nil
}

// --- json-rpc plumbing ---

func (s *Server) reply(id json.RawMessage, result any) {
	if len(id) == 0 {
		return // notification: no response
	}
	s.out.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) replyErr(id json.RawMessage, code int, msg string) {
	s.out.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func argStr(args map[string]any, k string) string {
	if v, ok := args[k].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, k string, def int) int {
	switch v := args[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
