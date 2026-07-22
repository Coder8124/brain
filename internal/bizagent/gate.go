package bizagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pragun/brain/internal/action"
)

// The gate tool: how the agent asks to do something outbound. It never acts —
// it enqueues a confirmation with a preview, and the effect only happens when
// the user approves it in the trust loop. This is the boundary between an
// assistant that prepares work and one that takes it into the world.

// RegisterGate adds the request_action tool. Outbound-capable agents get this;
// read/analyse-only ones do not, so an agent physically cannot act without it.
func RegisterGate(r *Registry) {
	r.Register(funcTool{
		name: "request_action",
		desc: "Request an OUTBOUND action that changes the outside world — send an email, export a file, book something. This does NOT do it: it queues the action with a preview for the user to approve. Use for anything that leaves the vault or spends/sends. Args: kind (send_email|export_file), title (one line), preview (exactly what will happen), and the action's fields (to, subject, body, path, content, …).",
		schema: objSchema(map[string]any{
			"kind":    strSchema("send_email or export_file"),
			"title":   strSchema("one-line summary of the action"),
			"preview": strSchema("exactly what will happen, for the user to read"),
			"to":      strSchema("email recipient (for send_email)"),
			"subject": strSchema("email subject (for send_email)"),
			"body":    strSchema("email body (for send_email)"),
			"path":    strSchema("destination path (for export_file)"),
			"content": strSchema("file content (for export_file)"),
		}, "kind", "title", "preview"),
		run: func(env *Env, args map[string]any) (string, error) {
			if env.DB == nil {
				return "", fmt.Errorf("no action queue available")
			}
			if err := action.Init(env.DB); err != nil {
				return "", err
			}
			payload := map[string]string{}
			for _, k := range []string{"to", "subject", "body", "path", "content"} {
				if v := strArg(args, k); v != "" {
					payload[k] = v
				}
			}
			a := &action.Action{
				Kind:    strArg(args, "kind"),
				Title:   strArg(args, "title"),
				Preview: strArg(args, "preview"),
				Payload: payload,
			}
			if err := action.Enqueue(env.DB, a); err != nil {
				return "", err
			}
			return fmt.Sprintf("Queued action #%d for your approval — it will NOT run until you confirm it. (%s)", a.ID, a.Title), nil
		},
	})
}

// RegisterDefaultExecutors wires the concrete effects the gate can carry out.
// Real integrations (SMTP, a booking API) register here; these two are honest
// local stand-ins that actually write something, so the loop is demonstrable
// end to end rather than vaporware. vault anchors where "sent" mail is filed.
func RegisterDefaultExecutors(vault string) {
	// send_email: with no SMTP configured, an approved email is written to an
	// outbox rather than silently dropped — a real, inspectable artifact, and
	// the exact seam a real mailer slots into later.
	action.Register("send_email", func(p map[string]string) (string, error) {
		dir := filepath.Join(vault, "business", "outbox")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		name := fmt.Sprintf("%d-%s.eml", time.Now().Unix(), slugifyGate(p["subject"]))
		path := filepath.Join(dir, name)
		msg := fmt.Sprintf("To: %s\nSubject: %s\n\n%s\n", p["to"], p["subject"], p["body"])
		if err := os.WriteFile(path, []byte(msg), 0o644); err != nil {
			return "", err
		}
		return "email queued to outbox: " + path + " (connect SMTP to send for real)", nil
	})

	// export_file: writes content to a path the user chose — an outbound effect
	// (it leaves the vault) and so it is gated.
	action.Register("export_file", func(p map[string]string) (string, error) {
		path := p["path"]
		if path == "" {
			return "", fmt.Errorf("export needs a destination path")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(p["content"]), 0o644); err != nil {
			return "", err
		}
		return "exported to " + path, nil
	})
}

func slugifyGate(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "message"
	}
	return out
}
