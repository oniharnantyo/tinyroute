package translate_test

import (
	"testing"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/translate"
)

type dummyReq struct{ name string }

func (d dummyReq) TranslateRequest(body []byte, state *core.StreamState) ([]byte, error) {
	return append(body, []byte("-"+d.name)...), nil
}

type dummyResp struct{ name string }

func (d dummyResp) TranslateResponse(chunk []byte, state *core.StreamState) ([][]byte, error) {
	return [][]byte{append(chunk, []byte("-"+d.name)...)}, nil
}

func TestRegistry(t *testing.T) {
	// Register direct pair
	translate.Register("dialectA", "openai", dummyReq{"AtoO"}, dummyResp{"AtoO"})
	translate.Register("openai", "dialectB", dummyReq{"OtoB"}, dummyResp{"OtoB"})

	// Direct lookup
	req, resp, ok := translate.Lookup("dialectA", "openai")
	if !ok || req == nil || resp == nil {
		t.Fatalf("expected direct lookup success for dialectA->openai")
	}

	// Composed lookup: dialectA -> openai -> dialectB
	cReq, cResp, cOk := translate.Lookup("dialectA", "dialectB")
	if !cOk || cReq == nil || cResp == nil {
		t.Fatalf("expected composed lookup success for dialectA->dialectB")
	}

	outReq, err := cReq.TranslateRequest([]byte("hello"), nil)
	if err != nil || string(outReq) != "hello-AtoO-OtoB" {
		t.Errorf("unexpected composed req output: %s, err: %v", string(outReq), err)
	}

	state := translate.NewStreamState()
	outResp, err := cResp.TranslateResponse([]byte("chunk"), state)
	if err != nil || len(outResp) != 1 || string(outResp[0]) != "chunk-OtoB-AtoO" {
		t.Errorf("unexpected composed resp output: %v, err: %v", string(outResp[0]), err)
	}

	// Unknown pair
	_, _, uOk := translate.Lookup("unknown1", "unknown2")
	if uOk {
		t.Errorf("expected ok=false for unknown pair")
	}

	// NeedsTranslation
	if !translate.NeedsTranslation("anthropic", "openai") {
		t.Errorf("expected NeedsTranslation(anthropic, openai) == true")
	}
	if translate.NeedsTranslation("openai", "openai") {
		t.Errorf("expected NeedsTranslation(openai, openai) == false")
	}

	// Reverse lookup should not match when only dialectA->openai is registered
	_, _, revOk := translate.Lookup("openai", "dialectA")
	if revOk {
		t.Errorf("expected ok=false for unregistered reverse pair openai->dialectA")
	}
	if translate.CanTranslate("openai", "dialectA") {
		t.Errorf("expected CanTranslate(openai, dialectA) == false")
	}

	// CanTranslate
	if !translate.CanTranslate("dialectA", "dialectB") {
		t.Errorf("expected CanTranslate(dialectA, dialectB) == true")
	}
	if translate.CanTranslate("dialectA", "unknown2") {
		t.Errorf("expected CanTranslate(dialectA, unknown2) == false")
	}
	if translate.CanTranslate("openai", "openai") {
		t.Errorf("expected CanTranslate(openai, openai) == false")
	}
}
