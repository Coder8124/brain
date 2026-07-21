package tutor

import (
	"strings"

	"github.com/pragun/brain/internal/router"
)

// Idle help: when a student sits on a studious screen without input for a
// while, they are usually stuck. The tutor offers a hand, and — only if they
// say yes — reads the screen and helps.
//
// The consent step is not optional. Reading the screen and running a model on
// it the moment someone pauses would be both creepy and wrong (a pause is often
// just thinking). The offer is cheap; the help is deliberate.

// ShouldOfferHelp decides whether to surface the "need help?" prompt.
//
// Two conditions, both required: the user has been idle past the threshold, and
// what is on screen is actually study material. Idle on a video or a chat is
// not someone stuck on a problem.
func ShouldOfferHelp(idleSeconds float64, threshold float64, screenText string) bool {
	return idleSeconds >= threshold && LooksStudious(screenText)
}

// Help produces tutoring guidance for what is on the screen.
//
// It coaches rather than answers: the goal is to unstick a learner, not to do
// the problem for them. A tutor that hands over the solution teaches nothing,
// so the prompt asks for the next step and the idea behind it, not the final
// answer.
func Help(rt *router.Router, screenText string) (string, error) {
	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}

	const system = "You are a patient tutor helping a student who appears stuck on what is on " +
		"their screen. Do not give the final answer. Identify what they are working on, name the " +
		"concept or step they are likely missing, and give one concrete nudge toward the next move. " +
		"Two or three sentences. Encouraging, not condescending."

	out, err := rt.Local().Chat(model, system, truncate(screenText, 4000), nil)
	return strings.TrimSpace(out), err
}
