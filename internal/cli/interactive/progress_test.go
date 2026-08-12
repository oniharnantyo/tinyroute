package interactive

import (
	"testing"
)

func TestSpinnerNonInteractive(t *testing.T) {
	falseVal := false
	SetCanPromptOverride(&falseVal)
	defer SetCanPromptOverride(nil)

	s, err := StartSpinner("Testing...")
	if err != nil {
		t.Fatalf("unexpected error starting spinner: %v", err)
	}
	s.Update("Updated...")
	s.Success("Done")
	s.Stop()
}

func TestProgressbarNonInteractive(t *testing.T) {
	falseVal := false
	SetCanPromptOverride(&falseVal)
	defer SetCanPromptOverride(nil)

	pb, err := StartProgressbar(10, "Progress")
	if err != nil {
		t.Fatalf("unexpected error starting progressbar: %v", err)
	}
	pb.Increment()
	pb.Update(5)
	pb.Stop()
}
