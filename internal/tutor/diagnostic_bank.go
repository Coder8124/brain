package tutor

import "strings"

// Verified question banks for the preset subjects.
//
// Small local models are not reliable at getting math and science answers
// *correct* — they produce plausible MCQs with the wrong option marked right,
// which for a diagnostic is worse than useless: it mismeasures the student and
// then seeds the review deck with false facts. So the presets ship a curated,
// human-verified bank. Model generation (diagnostic.go) is the fallback for
// arbitrary subjects only, where best-effort is the honest ceiling.
//
// Questions lean conceptual: placement is about what you understand, and
// concept checks are both better diagnostics and unambiguous to verify.

func q(subskill, question string, correct string, distractors ...string) MCQ {
	options := append([]string{correct}, distractors...)
	// The correct answer is authored first; scramble deterministically per
	// question so the right choice is not always option A, without needing RNG.
	shift := len(question) % len(options)
	rotated := make([]string, len(options))
	for i, o := range options {
		rotated[(i+shift)%len(options)] = o
	}
	correctIdx := shift % len(options)
	return MCQ{Subskill: subskill, Q: question, Options: rotated, Correct: correctIdx}
}

// bank maps a preset subject name to its verified questions.
var bank = map[string][]MCQ{
	"AP Calculus BC": {
		q("limits and continuity", "A function f is continuous at x = a when:",
			"the limit of f(x) as x→a equals f(a)",
			"f(a) = 0", "f is increasing at a", "f has a vertical asymptote at a"),
		q("limits and continuity", "What is the limit of sin(x)/x as x → 0?",
			"1", "0", "infinity", "the limit does not exist"),
		q("differentiation and its rules", "By the power rule, d/dx of x^n is:",
			"n·x^(n-1)", "x^(n-1)", "n·x^n", "x^(n+1)/(n+1)"),
		q("differentiation and its rules", "The derivative of a function at a point gives:",
			"the slope of the tangent line there", "the area under the curve", "the average value", "the concavity"),
		q("applications of derivatives", "At a local maximum of a differentiable function, f'(x) is:",
			"0", "positive", "negative", "undefined"),
		q("applications of derivatives", "If f''(x) > 0 on an interval, the graph there is:",
			"concave up", "concave down", "linear", "decreasing"),
		q("integration and accumulation", "The indefinite integral ∫ x dx equals:",
			"x²/2 + C", "x + C", "x² + C", "1 + C"),
		q("integration and accumulation", "The Fundamental Theorem of Calculus connects:",
			"differentiation and integration", "limits and continuity", "sequences and series", "algebra and geometry"),
		q("differential equations", "The general solution of dy/dx = k·y is:",
			"y = C·e^(kx)", "y = kx + C", "y = C·x^k", "y = k·e^(Cx)"),
		q("differential equations", "A slope field shows, at each point, the value of:",
			"dy/dx", "the area under the curve", "the second derivative", "the antiderivative"),
		q("infinite sequences and series", "The geometric series Σ r^n converges exactly when:",
			"|r| < 1", "|r| > 1", "r = 1", "r > 0"),
		q("infinite sequences and series", "The nth-term test shows a series diverges if:",
			"the terms do not approach 0", "the terms are positive", "the series is geometric", "the terms approach 0"),
	},
	"AP Physics C": {
		q("kinematics", "Ignoring air resistance, an object in free fall has acceleration:",
			"about 9.8 m/s² downward", "zero", "increasing with time", "dependent on its mass"),
		q("kinematics", "The area under a velocity–time graph represents:",
			"displacement", "acceleration", "force", "jerk"),
		q("Newton's laws and forces", "Newton's second law is written as:",
			"F = m·a", "F = m·v", "F = m/a", "F = m·a²"),
		q("Newton's laws and forces", "An object moving at constant velocity has a net force of:",
			"zero", "m·g", "m·a", "its weight"),
		q("work, energy, and momentum", "The work–energy theorem says net work equals the change in:",
			"kinetic energy", "momentum", "impulse", "power"),
		q("work, energy, and momentum", "Linear momentum is conserved when:",
			"no external force acts on the system", "energy is conserved", "the collision is elastic", "velocity is constant"),
		q("rotational motion", "The rotational analog of force is:",
			"torque", "angular velocity", "moment of inertia", "angular momentum"),
		q("rotational motion", "Torque from a given force is greatest when the force is applied:",
			"perpendicular to the lever arm", "parallel to the lever arm", "at the axis of rotation", "at a 45° angle"),
		q("electrostatics and circuits", "In Coulomb's law, the force between two charges is proportional to:",
			"1/r²", "1/r", "r", "r²"),
		q("electrostatics and circuits", "Ohm's law relates voltage, current, and resistance as:",
			"V = I·R", "V = I/R", "V = I²·R", "V = R/I"),
		q("magnetism and induction", "The magnetic force on a moving charge is greatest when its velocity is:",
			"perpendicular to the magnetic field", "parallel to the field", "zero", "antiparallel to the field"),
		q("magnetism and induction", "Faraday's law relates the induced EMF to:",
			"the rate of change of magnetic flux", "the magnetic field strength", "the current", "the resistance"),
	},
	"AP Chemistry": {
		q("atomic structure and periodicity", "An element's atomic number equals its number of:",
			"protons", "neutrons", "electrons plus neutrons", "total nucleons"),
		q("atomic structure and periodicity", "Moving left to right across a period, atomic radius generally:",
			"decreases", "increases", "stays constant", "doubles"),
		q("bonding and molecular geometry", "A bond between atoms with a large electronegativity difference is:",
			"ionic", "nonpolar covalent", "metallic", "a hydrogen bond"),
		q("bonding and molecular geometry", "A molecule with four bonding pairs and no lone pairs is:",
			"tetrahedral", "trigonal planar", "bent", "linear"),
		q("intermolecular forces", "Among these, the strongest intermolecular force is:",
			"hydrogen bonding", "London dispersion forces", "dipole–dipole forces", "none of these"),
		q("intermolecular forces", "Which substance has the highest boiling point?",
			"H₂O", "H₂S", "HCl", "CH₄"),
		q("stoichiometry and reactions", "One mole of any substance contains:",
			"6.022×10²³ particles", "12 grams", "22.4 particles", "100 particles"),
		q("stoichiometry and reactions", "In a balanced chemical equation, what is conserved?",
			"mass and the number of each atom", "only the volume", "only moles of gas", "temperature"),
		q("kinetics", "Raising the temperature generally makes the reaction rate:",
			"increase", "decrease", "stay the same", "become zero"),
		q("kinetics", "A catalyst speeds a reaction by:",
			"lowering the activation energy", "raising the temperature", "shifting the equilibrium", "adding more reactant"),
		q("thermodynamics and equilibrium", "A reaction is spontaneous when its ΔG is:",
			"negative", "positive", "zero", "equal to ΔH"),
		q("thermodynamics and equilibrium", "By Le Chatelier's principle, adding reactant shifts equilibrium:",
			"toward products", "toward reactants", "not at all", "to completion instantly"),
	},
}

// bankFor returns the verified questions for a preset subject, if any, filtered
// to the requested subskills.
func bankFor(subject string, subskills []string) ([]MCQ, bool) {
	items, ok := bank[subject]
	if !ok {
		return nil, false
	}
	want := map[string]bool{}
	for _, s := range subskills {
		want[strings.ToLower(s)] = true
	}
	var out []MCQ
	for _, q := range items {
		if len(want) == 0 || want[strings.ToLower(q.Subskill)] {
			out = append(out, q)
		}
	}
	return out, len(out) > 0
}
