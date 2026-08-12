package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
)

type DefaultFusionRunner struct {
	Deps *Deps
}

func (fr *DefaultFusionRunner) RunPool(ctx context.Context, hops []core.Hop, body []byte) (*core.ProxyResult, error) {
	if len(hops) == 0 {
		return nil, fmt.Errorf("no hops provided for pool execution")
	}

	type poolRes struct {
		result *core.ProxyResult
		err    error
	}

	resCh := make(chan poolRes, len(hops))
	ctxPool, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, hop := range hops {
		wg.Add(1)
		go func(h core.Hop) {
			defer wg.Done()
			res, err := fr.executeSingleHop(ctxPool, h, body)
			if err == nil && res.Outcome == core.OutcomeOK {
				select {
				case resCh <- poolRes{result: res}:
					cancel()
				default:
				}
			} else {
				select {
				case resCh <- poolRes{err: err}:
				default:
				}
			}
		}(hop)
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	var lastErr error
	for r := range resCh {
		if r.result != nil && r.result.Outcome == core.OutcomeOK {
			return r.result, nil
		}
		if r.err != nil {
			lastErr = r.err
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("all pool hops failed")
}

func (fr *DefaultFusionRunner) RunFused(ctx context.Context, hops []core.Hop, body []byte) (*core.ProxyResult, error) {
	if len(hops) == 0 {
		return nil, fmt.Errorf("no hops provided for fused execution")
	}

	quorum := (len(hops) / 2) + 1
	type hopRes struct {
		hop    core.Hop
		result *core.ProxyResult
		err    error
	}

	resCh := make(chan hopRes, len(hops))
	var wg sync.WaitGroup

	for _, hop := range hops {
		wg.Add(1)
		go func(h core.Hop) {
			defer wg.Done()
			res, err := fr.executeSingleHop(ctx, h, body)
			resCh <- hopRes{hop: h, result: res, err: err}
		}(hop)
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	var successes []*core.ProxyResult
	var attempts []core.Attempt

	for r := range resCh {
		if r.result != nil {
			attempts = append(attempts, r.result.Attempts...)
			if r.result.Outcome == core.OutcomeOK {
				successes = append(successes, r.result)
			}
		}
	}

	if len(successes) < quorum {
		return nil, fmt.Errorf("fusion quorum failed: got %d successes, needed %d", len(successes), quorum)
	}

	// Synthesize output from quorum responses
	winner := successes[0]
	if len(successes) > 1 {
		var combined []json.RawMessage
		for _, s := range successes {
			if len(s.RespBody) > 0 {
				combined = append(combined, json.RawMessage(s.RespBody))
			}
		}
		if synth, err := json.Marshal(map[string]interface{}{
			"fusion":       "quorum_synthesis",
			"quorum":       quorum,
			"responses":    combined,
			"primary_resp": json.RawMessage(winner.RespBody),
		}); err == nil {
			winner.RespBody = synth
		}
	}
	winner.Attempts = attempts
	return winner, nil
}

func (fr *DefaultFusionRunner) executeSingleHop(ctx context.Context, hop core.Hop, body []byte) (*core.ProxyResult, error) {
	prov, ok := fr.Deps.GetProvider(hop.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", hop.Provider)
	}
	hopDialect, ok := fr.Deps.GetDialect(prov.Dialect)
	if !ok {
		return nil, fmt.Errorf("unknown dialect %q", prov.Dialect)
	}

	outboundPaths := hopDialect.Paths()
	if len(outboundPaths) == 0 {
		return nil, fmt.Errorf("no outbound paths for dialect %q", prov.Dialect)
	}
	url := JoinURL(prov.BaseURL, outboundPaths[0])

	rewrittenBody, err := hopDialect.RewriteModel(body, hop.Model)
	if err != nil {
		return nil, err
	}

	accountKey := hop.HopKey()
	outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rewrittenBody))
	if err != nil {
		return nil, err
	}

	var credStrategy credential.Credential = prov.Credential
	if hop.Account != "" && hop.Account != "default" && prov.Accounts != nil {
		if c, ok := prov.Accounts[hop.Account]; ok {
			credStrategy = c
		}
	}

	var tokRes credential.TokenResult
	if credStrategy != nil {
		tokRes, _ = credStrategy.Token(ctx)
	} else if prov.APIKey != "" {
		tokRes = credential.TokenResult{Value: prov.APIKey, Kind: credential.KindStatic}
	}

	outReq.Header = hopDialect.AuthHeaders(tokRes, prov.Headers)

	if strings.Contains(prov.BaseURL, "cline.bot") {
		if auth := outReq.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") && !strings.HasPrefix(auth, "Bearer workos:") {
			outReq.Header.Set("Authorization", "Bearer workos:"+strings.TrimPrefix(auth, "Bearer "))
		}
		if outReq.Header.Get("HTTP-Referer") == "" {
			outReq.Header.Set("HTTP-Referer", "https://cline.bot")
		}
		if outReq.Header.Get("X-Title") == "" {
			outReq.Header.Set("X-Title", "Cline")
		}
		if outReq.Header.Get("X-CLIENT-TYPE") == "" {
			outReq.Header.Set("X-CLIENT-TYPE", "extension")
		}
		if outReq.Header.Get("X-IS-MULTIROOT") == "" {
			outReq.Header.Set("X-IS-MULTIROOT", "false")
		}
		if outReq.Header.Get("User-Agent") == "" {
			outReq.Header.Set("User-Agent", "Cline/3.54.0")
		}
		if outReq.Header.Get("X-CLIENT-VERSION") == "" {
			outReq.Header.Set("X-CLIENT-VERSION", "3.54.0")
		}
		if outReq.Header.Get("X-CORE-VERSION") == "" {
			outReq.Header.Set("X-CORE-VERSION", "3.54.0")
		}
	}

	start := time.Now()
	resp, err := fr.Deps.Transport.RoundTrip(outReq)
	elapsed := time.Since(start)

	attempt := core.Attempt{
		Provider: accountKey,
		Model:    hop.Model,
		Elapsed:  elapsed,
	}

	if err != nil {
		attempt.Status = 0
		return &core.ProxyResult{Attempts: []core.Attempt{attempt}, Outcome: core.OutcomeChainExhausted}, err
	}
	defer resp.Body.Close()

	attempt.Status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &core.ProxyResult{Attempts: []core.Attempt{attempt}, Outcome: core.OutcomeChainExhausted}, fmt.Errorf("upstream error %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	return &core.ProxyResult{
		Attempts: []core.Attempt{attempt},
		Outcome:  core.OutcomeOK,
		ReqBody:  rewrittenBody,
		RespBody: respBody,
	}, nil
}
