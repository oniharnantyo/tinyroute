package interactive

import (
	"testing"
)

func TestWizardNonInteractive(t *testing.T) {
	falseVal := false
	SetCanPromptOverride(&falseVal)
	defer SetCanPromptOverride(nil)

	err := RunInitWizard()
	if err != nil {
		t.Fatalf("unexpected error running wizard non-interactively: %v", err)
	}
}
