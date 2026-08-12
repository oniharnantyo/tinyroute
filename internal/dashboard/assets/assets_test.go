package assets

import "testing"

func TestAssetsEmbedding(t *testing.T) {
	if FS() == nil {
		t.Errorf("expected non-nil FS")
	}

	cssData, err := ReadFile("styles.css")
	if err != nil || len(cssData) == 0 {
		t.Errorf("expected non-empty styles.css: %v", err)
	}

	anthropicLogo, ok := LogoSVG("anthropic")
	if !ok || len(anthropicLogo) == 0 {
		t.Errorf("expected anthropic.svg to be embedded")
	}

	_, ok = LogoSVG("nonexistent_provider_foo")
	if ok {
		t.Errorf("expected false for nonexistent provider logo")
	}
}
