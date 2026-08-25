package switchcomp

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSwitchRendering(t *testing.T) {
	trueVal := true
	falseVal := false

	// 1. Checked switch
	buf := new(bytes.Buffer)
	err := Switch(Props{
		Checked: &trueVal,
		Name:    "active",
		Value:   "true",
	}).Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render checked switch: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `role="switch"`) {
		t.Errorf("expected role=\"switch\", got: %s", out)
	}
	if !strings.Contains(out, `aria-checked="true"`) {
		t.Errorf("expected aria-checked=\"true\", got: %s", out)
	}
	if !strings.Contains(out, `data-checked`) {
		t.Errorf("expected data-checked, got: %s", out)
	}
	if !strings.Contains(out, `type="submit"`) {
		t.Errorf("expected type=\"submit\", got: %s", out)
	}

	// 2. Unchecked switch
	buf.Reset()
	err = Switch(Props{
		Checked: &falseVal,
	}).Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render unchecked switch: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, `aria-checked="false"`) {
		t.Errorf("expected aria-checked=\"false\", got: %s", out)
	}
	if !strings.Contains(out, `data-unchecked`) {
		t.Errorf("expected data-unchecked, got: %s", out)
	}

	// 3. Disabled switch
	buf.Reset()
	err = Switch(Props{
		Disabled: true,
	}).Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("failed to render disabled switch: %v", err)
	}
	out = buf.String()
	if !strings.Contains(out, `disabled`) || !strings.Contains(out, `aria-disabled="true"`) {
		t.Errorf("expected disabled attribute and aria-disabled, got: %s", out)
	}
}
