// Package tutor turns the vault into study material.
//
// Where the secretary tells you what to do, the tutor asks you what you know.
// It summarises what you have been reading and generates active-recall
// questions from it — and, with screen notes on, quietly captures what you
// study so there is material to be quizzed on.
package tutor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pragun/brain/internal/index"
	"github.com/pragun/brain/internal/provider"
	"github.com/pragun/brain/internal/router"
)

// Card is one active-recall question. Answer is kept separate so the UI can
// hide it — recall only works if you try before you see it.
type Card struct {
	Q      string `json:"q"`
	A      string `json:"a"`
	Source string `json:"source"` // note slug the card came from
}

var cardSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"cards": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "string"},
					"a": map[string]any{"type": "string"},
				},
				"required":             []string{"q", "a"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"cards"},
	"additionalProperties": false,
}

// Questions generates recall cards from the notes most relevant to a topic.
//
// Retrieval first, then generation: the cards are grounded in what is actually
// in the vault, so a tutor cannot quiz you on things you never studied. The
// answer to every card is present in the source note, which is what makes the
// cards checkable rather than the model's opinion.
func Questions(ix *index.Index, rt *router.Router, topic string, n int) ([]Card, error) {
	embed, _ := rt.Model(router.T0)
	hits, err := ix.HybridSearch(rt.Local(), embed, topic, 4)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("nothing in the vault about %q to study yet", topic)
	}

	model, err := rt.ModelFor(router.T2, true)
	if err != nil {
		return nil, err
	}

	var cards []Card
	for _, h := range hits {
		if len(cards) >= n {
			break
		}
		system := fmt.Sprintf("You are a tutor. From the note below, write up to %d active-recall "+
			"questions that test understanding, not trivia. Reply with JSON only. Every answer must "+
			"be stated in or directly inferable from the note — never ask about something the note "+
			"does not contain. Prefer 'why' and 'how' over 'what'.", (n+len(hits)-1)/len(hits))

		out, err := rt.Local().Chat(model, system, h.Title+"\n\n"+h.Body, cardSchema)
		if err != nil {
			continue // one bad note should not sink the whole set
		}
		var res struct {
			Cards []Card `json:"cards"`
		}
		if json.Unmarshal([]byte(cleanJSON(out)), &res) != nil {
			continue
		}
		for _, c := range res.Cards {
			if strings.TrimSpace(c.Q) == "" || strings.TrimSpace(c.A) == "" {
				continue
			}
			c.Source = h.Slug
			cards = append(cards, c)
			if len(cards) >= n {
				break
			}
		}
	}

	if len(cards) == 0 {
		return nil, fmt.Errorf("could not generate questions from the notes on %q", topic)
	}
	return cards, nil
}

// Summarize gives a study-oriented digest of what the vault holds on a topic —
// the "what should I review" view, distinct from the secretary's "what should I
// do".
func Summarize(ix *index.Index, rt *router.Router, topic string) (string, []string, error) {
	embed, _ := rt.Model(router.T0)
	hits, err := ix.HybridSearch(rt.Local(), embed, topic, 6)
	if err != nil {
		return "", nil, err
	}
	if len(hits) == 0 {
		return "", nil, fmt.Errorf("nothing in the vault about %q yet", topic)
	}

	model, _ := rt.Model(router.T2)

	var ctx strings.Builder
	var sources []string
	for _, h := range hits {
		chunk := fmt.Sprintf("## %s\n%s\n\n", h.Title, strings.TrimSpace(h.Body))
		if ctx.Len()+len(chunk) > 6000 {
			break
		}
		ctx.WriteString(chunk)
		sources = append(sources, h.Slug)
	}

	system := "You are a study tutor. Summarise what these notes cover as a tight revision digest: " +
		"the key ideas and how they connect, in a few bullet points. State only what the notes support. " +
		"End with one sentence naming the biggest gap worth studying next."

	answer, err := rt.Local().Chat(model, system, ctx.String(), nil)
	return strings.TrimSpace(answer), sources, err
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

var _ = provider.Provider{}
