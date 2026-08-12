package translate

import (
	"sync"

	"github.com/oniharnantyo/tinyroute/internal/core"
)

// canonical is the intermediate dialect through which the registry pivots for
// composed (two-hop) translations. Exact source→target pairs run as direct
// routes; anything else composes from→openai→to.
const canonical = "openai"

// pair keys a registered translator (req and/or resp may be nil).
type pair struct {
	req  core.RequestTranslator
	resp core.ResponseTranslator
}

var (
	mu   sync.RWMutex
	regs = map[string]pair{}
)

// Register registers translators for a (from, to) dialect pair. req or resp may
// be nil when a pair is only used in a single direction (a nil value simply
// means that direction is unavailable on the direct route).
func Register(from, to string, req core.RequestTranslator, resp core.ResponseTranslator) {
	mu.Lock()
	defer mu.Unlock()
	key := from + "\x00" + to
	p := regs[key]
	if req != nil {
		p.req = req
	}
	if resp != nil {
		p.resp = resp
	}
	regs[key] = p
}

// Lookup resolves translators for a (from, to) pair. It returns a direct pair
// when one is registered, else composes from→openai→to. Returns ok=false when
// no path exists (the caller treats this as "cross-dialect routing
// unavailable").
func Lookup(from, to string) (req core.RequestTranslator, resp core.ResponseTranslator, ok bool) {
	mu.RLock()
	direct, found := regs[from+"\x00"+to]
	mu.RUnlock()

	if found {
		return direct.req, direct.resp, direct.req != nil || direct.resp != nil
	}

	// Compose via the canonical intermediate format: from→openai→to.
	if from == canonical || to == canonical {
		return nil, nil, false
	}
	req = composeReq(from, to)
	resp = composeResp(from, to)
	if req == nil && resp == nil {
		return nil, nil, false
	}
	return req, resp, true
}

func composeReq(from, to string) core.RequestTranslator {
	req1, _, ok1 := Lookup(from, canonical)
	if !ok1 || req1 == nil {
		return nil
	}
	req2, _, ok2 := Lookup(canonical, to)
	if !ok2 || req2 == nil {
		return nil
	}
	return &composedReq{a: req1, b: req2}
}

func composeResp(from, to string) core.ResponseTranslator {
	_, resp1, ok1 := Lookup(canonical, to)
	if !ok1 || resp1 == nil {
		return nil
	}
	_, resp2, ok2 := Lookup(from, canonical)
	if !ok2 || resp2 == nil {
		return nil
	}
	return &composedResp{a: resp1, b: resp2}
}

// composedReq chains two request translators: from→openai then openai→to.
type composedReq struct {
	a core.RequestTranslator // from → openai
	b core.RequestTranslator // openai → to
}

func (c *composedReq) TranslateRequest(body []byte, state *StreamState) ([]byte, error) {
	mid, err := c.a.TranslateRequest(body, state)
	if err != nil {
		return nil, err
	}
	return c.b.TranslateRequest(mid, state)
}

// composedResp chains two response translators: from→openai then openai→to.
type composedResp struct {
	a core.ResponseTranslator // from → openai
	b core.ResponseTranslator // openai → to
}

func (c *composedResp) TranslateResponse(chunk []byte, state *StreamState) ([][]byte, error) {
	mid, err := c.a.TranslateResponse(chunk, state)
	if err != nil {
		return nil, err
	}
	var frames [][]byte
	for _, m := range mid {
		f, err := c.b.TranslateResponse(m, state)
		if err != nil {
			return nil, err
		}
		frames = append(frames, f...)
	}
	if chunk == nil {
		f, err := c.b.TranslateResponse(nil, state)
		if err != nil {
			return nil, err
		}
		frames = append(frames, f...)
	}
	return frames, nil
}

// NeedsTranslation reports whether two dialects differ (i.e. a translation hop
// is required). It is the proxy's gate: dialects that match pass through.
func NeedsTranslation(from, to string) bool { return from != to }

// CanTranslate reports whether a cross-dialect translation path exists for the
// pair. It is false when the dialects match or when no translator path is
// registered. It is the single predicate consulted by both route resolution and
// the proxy (a single source of truth backed by translate.Lookup).
func CanTranslate(from, to string) bool {
	if from == to {
		return false
	}
	_, _, ok := Lookup(from, to)
	return ok
}
