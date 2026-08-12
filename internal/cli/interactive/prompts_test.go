package interactive

import (
	"testing"
)

func TestCanPrompt(t *testing.T) {
	// Test override
	trueVal := true
	SetCanPromptOverride(&trueVal)
	if !CanPrompt() {
		t.Errorf("expected CanPrompt() to be true when override is true")
	}

	falseVal := false
	SetCanPromptOverride(&falseVal)
	if CanPrompt() {
		t.Errorf("expected CanPrompt() to be false when override is false")
	}

	SetCanPromptOverride(nil)
}

func TestConfirmNonInteractive(t *testing.T) {
	falseVal := false
	SetCanPromptOverride(&falseVal)
	defer SetCanPromptOverride(nil)

	res, err := Confirm("Continue?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res {
		t.Errorf("expected true, got %v", res)
	}

	res, err = Confirm("Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res {
		t.Errorf("expected false, got %v", res)
	}
}

func TestInputNonInteractive(t *testing.T) {
	falseVal := false
	SetCanPromptOverride(&falseVal)
	defer SetCanPromptOverride(nil)

	res, err := Input("Enter name", "default_name", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "default_name" {
		t.Errorf("expected default_name, got %q", res)
	}
}

func TestSelectNonInteractive(t *testing.T) {
	falseVal := false
	SetCanPromptOverride(&falseVal)
	defer SetCanPromptOverride(nil)

	options := []string{"option1", "option2"}
	res, err := Select("Choose option", options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "option1" {
		t.Errorf("expected option1, got %q", res)
	}

	_, err = Select("Choose option", []string{})
	if err == nil {
		t.Errorf("expected error when options list is empty")
	}
}
