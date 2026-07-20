// Package provider is one client for every model runtime.
//
// Ollama, LM Studio, Jan and Msty all speak OpenAI-compatible /v1, so the only
// thing that differs is the port and which models happen to be loaded. Cloud
// BYOK is the same surface with a base URL and a key.
package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// LocalEndpoint is a runtime we know how to find without configuration.
type LocalEndpoint struct {
	Name string
	URL  string
}

// Probed in this order; first match wins when nothing is configured.
var LocalEndpoints = []LocalEndpoint{
	{"Ollama", "http://localhost:11434/v1"},
	{"LM Studio", "http://localhost:1234/v1"},
	{"Jan", "http://localhost:1337/v1"},
	{"Msty", "http://localhost:10000/v1"},
}

type Provider struct {
	Name    string
	BaseURL string
	APIKey  string
	http    *http.Client
}

func New(name, baseURL, apiKey string) *Provider {
	return &Provider{
		Name:    name,
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		// Local generation on a 24B model genuinely takes a while.
		http: &http.Client{Timeout: 300 * time.Second},
	}
}

type Discovered struct {
	Provider *Provider
	Models   []string
}

// Discover probes the well-known ports and reports what is actually running.
// This is what lets the app say "found LM Studio running Qwen3" on first launch
// instead of presenting an empty endpoint configuration box.
func Discover() []Discovered {
	probe := &http.Client{Timeout: 400 * time.Millisecond}
	var out []Discovered

	for _, ep := range LocalEndpoints {
		res, err := probe.Get(ep.URL + "/models")
		if err != nil {
			continue
		}
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		err = json.NewDecoder(res.Body).Decode(&list)
		res.Body.Close()
		if err != nil {
			continue
		}
		models := make([]string, 0, len(list.Data))
		for _, m := range list.Data {
			models = append(models, m.ID)
		}
		out = append(out, Discovered{New(ep.Name, ep.URL, ""), models})
	}
	return out
}

func (p *Provider) post(path string, body any, into any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", p.BaseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	res, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s unreachable at %s: %w", p.Name, p.BaseURL, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var detail bytes.Buffer
		detail.ReadFrom(res.Body)
		return fmt.Errorf("%s returned %s: %s", p.Name, res.Status, strings.TrimSpace(detail.String()))
	}
	return json.NewDecoder(res.Body).Decode(into)
}

func (p *Provider) Embed(model string, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	var res struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := p.post("/embeddings", map[string]any{"model": model, "input": inputs}, &res); err != nil {
		return nil, err
	}
	if len(res.Data) != len(inputs) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(inputs), len(res.Data))
	}

	out := make([][]float32, len(res.Data))
	for i, d := range res.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

// Chat runs a completion. A non-nil schema enables constrained decoding: small
// local models are unreliable at tool-calling but near-perfect when JSON is
// enforced at the sampler, which is why every extraction path goes through here
// rather than through tool definitions.
func (p *Provider) Chat(model, system, user string, schema map[string]any) (string, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
		"stream":      false,
	}
	if schema != nil {
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "out",
				"strict": true,
				"schema": schema,
			},
		}
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := p.post("/chat/completions", body, &res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return res.Choices[0].Message.Content, nil
}
