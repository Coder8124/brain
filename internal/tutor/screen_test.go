package tutor

import "testing"

func TestLooksStudiousRequiresRealSignal(t *testing.T) {
	study := `Chapter 4: Eigenvalues and Eigenvectors. Definition: an eigenvector of a
	linear transformation is a nonzero vector that changes at most by a scalar factor.
	The theorem states that for a square matrix the determinant of A minus lambda I
	equals zero. Worked example: solve the characteristic equation. Exercise 4.1 asks
	you to find the eigenvalues of the given matrix. This derivation shows the proof.`
	if !LooksStudious(study) {
		t.Error("a textbook page with theorem/definition/exercise/proof should register as studious")
	}
}

func TestNonStudiousDomainsAreVetoed(t *testing.T) {
	// Has study-ish words but is clearly a chat — the domain veto must win.
	chat := `direct message from Sam: hey did you study the chapter yet? there was a
	definition and a theorem on the problem set. anyway lets grab food. chat feed
	notification inbox. example example example proof derivation study quiz.`
	if LooksStudious(chat) {
		t.Error("a chat window must be vetoed even when it mentions studious words")
	}
}

func TestShortTextIsNeverStudious(t *testing.T) {
	if LooksStudious("theorem proof definition") {
		t.Error("a few words is not a page of study material")
	}
}

func TestBankStatementIsNotStudious(t *testing.T) {
	bank := `Account balance summary. Your checking balance is shown below. Recent
	transactions and payments. Available balance. Password required to view full
	statement. This is a long enough block of text to pass the length gate easily
	but it is plainly financial and must never be captured as a study note at all.`
	if LooksStudious(bank) {
		t.Error("a bank statement must never be treated as study material")
	}
}

func TestShouldOfferHelpNeedsIdleAndStudious(t *testing.T) {
	study := `Chapter 4: Eigenvalues and eigenvectors. Definition of an eigenvector as a
	nonzero vector whose direction is preserved under a linear transformation. The theorem
	states a condition on the determinant, and its proof follows by expanding the
	characteristic polynomial. A worked example is given, followed by an exercise asking the
	student to solve the characteristic equation for the supplied matrix and find each
	eigenvalue. This derivation and figure together make a sufficiently long block of study
	material with several distinct cues, comfortably past the length gate this test needs.`

	if !ShouldOfferHelp(15, 12, study) {
		t.Error("idle past threshold on a study page should offer help")
	}
	if ShouldOfferHelp(3, 12, study) {
		t.Error("a brief pause must not trigger the offer")
	}
	if ShouldOfferHelp(30, 12, "just a youtube video feed playing in the background here") {
		t.Error("idle on non-study content must not offer help")
	}
}
