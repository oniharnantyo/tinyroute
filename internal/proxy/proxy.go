// Package proxy implements the attempt loop and SSE relay described in
// design decision D14 — the only orchestrator in tinyroute. It imports
// internal/core and the standard library only; every provider, dialect,
// health, selection, and recording concern arrives through Deps as an
// interface or function value.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/cloudcode"
	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/translate"
	"github.com/oniharnantyo/tinyroute/internal/translate/response"
)

// maxBodySize is the request body buffer cap (D2). Buffering is required
// because a consumed stream cannot be replayed to the next hop in a chain.
const maxBodySize = 32 << 20 // 32 MB

// ProviderInfo holds the runtime fields of a provider needed to attempt a
// hop. It mirrors config.Provider's shape without importing internal/config.
// AccountInfo holds per-account metadata.
type AccountInfo struct {
	Quota *core.QuotaConfig
}

var defaultCloudCodeOnboarding = cloudcode.NewOnboarding("", nil)

// ProviderInfo holds the runtime fields of a provider needed to attempt a
// hop. It mirrors config.Provider's shape without importing internal/config.
type ProviderInfo struct {
	Dialect     string
	BaseURL     string
	Transport   string
	APIKey      string
	Credential  credential.Credential
	Headers     map[string]*string
	Accounts    map[string]credential.Credential
	AccountObjs map[string]AccountInfo
	Selection   core.AccountStrategy
	StickyLimit int
	Quota       *core.QuotaConfig
}

// Deps holds every dependency the proxy needs, injected from main. No field
// here is a sibling internal package type; all are core types, stdlib
// types, or function values supplied by main's wiring.
type Deps struct {
	// Transport sends the outbound request for a hop. Its
	// ResponseHeaderTimeout bounds the failover window (D12); there is
	// deliberately no whole-request deadline so streaming bodies are
	// never cut off.
	Transport *http.Transport

	// GetProvider resolves a provider name (from a Hop) to its runtime info.
	GetProvider func(name string) (ProviderInfo, bool)

	// GetDialect resolves a dialect name (from a ProviderInfo) to its
	// core.Dialect implementation.
	GetDialect func(name string) (core.Dialect, bool)

	CloudCodeOnboarding *cloudcode.Onboarding

	Health       core.HealthStore
	ResetParser  core.ResetParser
	UsageStore   *core.UsageStore
	Affinity     core.Affinity
	Selector     core.Selector
	Recorder     core.Recorder
	FusionRunner core.FusionRunner

	Logger      *slog.Logger
	CaptureMode string // "full" or "metadata" (config.Service.Capture)
	InjectUsage bool
	Cooldown429 time.Duration
	Cooldown5xx time.Duration
	// NoPenalties suppresses health cooldowns for failures. Set on probe-scoped
	// Deps so an operator-initiated model test never takes a provider out of
	// rotation for live traffic. Existing cooldowns still gate hop selection.
	NoPenalties bool
}

// Handler returns the HTTP handler for proxied requests. It expects a
// *RequestCtx to already be attached to the request context (dialect
// resolution, route resolution, and body parsing happen upstream; auth has
// already run) — see D14's ordering. This handler owns only the attempt
// loop, SSE relay, and off-critical-path recording.
func Handler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlerStart := time.Now()
		reqCtx := getRequestContext(r.Context())
		if reqCtx == nil {
			if deps != nil && deps.Logger != nil {
				deps.Logger.Error("missing request context")
			}
			http.Error(w, "internal error: missing request context", http.StatusInternalServerError)
			return
		}
		inboundDialect := reqCtx.Dialect

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
		if err != nil {
			inboundDialect.WriteError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
			return
		}
		if len(body) > maxBodySize {
			inboundDialect.WriteError(w, http.StatusRequestEntityTooLarge, "invalid_request_error",
				fmt.Sprintf("request body exceeds %d MB limit", maxBodySize>>20))
			return
		}

		ctx := r.Context()
		parsed := reqCtx.Parsed
		resolved := reqCtx.Route

		availableHops := deps.Selector.Select(resolved.Hops, deps.Health.Available)

		var attempts []core.Attempt
		var finalUsage *core.Usage
		var respBody []byte
		var winningProvider string
		var winningXlatedReq []byte
		var winningRawResp []byte
		outcome := core.OutcomeChainExhausted
		committed := false

		if resolved.Mode == "pool" || resolved.Mode == "fused" {
			runner := deps.FusionRunner
			if runner == nil {
				runner = &DefaultFusionRunner{Deps: deps}
			}
			var res *core.ProxyResult
			var err error
			if resolved.Mode == "pool" {
				res, err = runner.RunPool(ctx, availableHops, body)
			} else {
				res, err = runner.RunFused(ctx, availableHops, body)
			}
			if err == nil && res != nil && res.Outcome == core.OutcomeOK {
				committed = true
				winningProvider = resolved.ComboName
				if winningProvider == "" {
					winningProvider = "combo"
				}
				winningXlatedReq = res.ReqBody
				winningRawResp = res.RespBody
				attempts = res.Attempts
				respBody = res.RespBody
				finalUsage = res.Usage
				outcome = core.OutcomeOK
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(respBody)
				goto record
			}
		}

	hopLoop:
		for _, hop := range availableHops {
			prov, ok := deps.GetProvider(hop.Provider)
			if !ok {
				continue
			}

			hopDialect, ok := deps.GetDialect(prov.Dialect)
			if !ok {
				continue
			}

			inboundName := inboundDialect.Name()
			hopName := prov.Dialect

			var reqTrans core.RequestTranslator
			var respTrans core.ResponseTranslator
			if translate.NeedsTranslation(inboundName, hopName) {
				reqTrans, respTrans, _ = translate.Lookup(inboundName, hopName)
			}

			state := translate.NewStreamState()

			xlatedBody := body
			if reqTrans != nil {
				var xErr error
				xlatedBody, xErr = reqTrans.TranslateRequest(body, state)
				if xErr != nil {
					continue
				}
			}

			rewrittenBody, err := hopDialect.RewriteModel(xlatedBody, hop.Model)
			if err != nil {
				continue
			}

			if deps.InjectUsage && parsed.Stream {
				if injected, ok := hopDialect.InjectUsageOption(rewrittenBody); ok {
					rewrittenBody = injected
				}
			}

			outboundPaths := hopDialect.Paths()
			if len(outboundPaths) == 0 {
				continue
			}
			url := JoinURL(prov.BaseURL, outboundPaths[0])

			// Resolve account candidates for this hop
			var accounts []string
			if hop.Account != "" && hop.Account != "default" {
				accounts = []string{hop.Account}
			} else if len(prov.Accounts) > 0 {
				var accNames []string
				for accName := range prov.Accounts {
					accNames = append(accNames, accName)
				}
				sort.Strings(accNames)
				accounts = core.SelectAccountsAffinity(accNames, prov.Selection, func(acc string) bool {
					accKey := hop.Provider + "/" + acc
					if !deps.Health.AvailableModel(accKey, hop.Model) {
						return false
					}
					var quota *core.QuotaConfig
					if accObj, ok := prov.AccountObjs[acc]; ok && accObj.Quota != nil {
						quota = accObj.Quota
					} else if prov.Quota != nil {
						quota = prov.Quota
					}
					if deps.UsageStore != nil && deps.UsageStore.Exhausted(accKey, quota) {
						return false
					}
					return true
				}, getProviderCounter(hop.Provider), deps.Affinity, prov.StickyLimit, hop.Provider)
			}
			if len(accounts) == 0 {
				accounts = []string{"default"}
			}

			for _, accName := range accounts {
				accountKey := hop.Provider
				if accName != "" && accName != "default" {
					accountKey = hop.Provider + "/" + accName
				}

				if !deps.Health.AvailableModel(accountKey, hop.Model) {
					continue
				}
				var quota *core.QuotaConfig
				if accObj, ok := prov.AccountObjs[accName]; ok && accObj.Quota != nil {
					quota = accObj.Quota
				} else if prov.Quota != nil {
					quota = prov.Quota
				}
				if deps.UsageStore != nil && deps.UsageStore.Exhausted(accountKey, quota) {
					continue
				}

				var credStrategy credential.Credential = prov.Credential
				if prov.Accounts != nil {
					if c, ok := prov.Accounts[accName]; ok {
						credStrategy = c
					}
				}

				var tokRes credential.TokenResult
				if credStrategy != nil {
					var tokErr error
					tokRes, tokErr = credStrategy.Token(ctx)
					if tokErr != nil {
						if deps.Logger != nil {
							deps.Logger.Error("failed to resolve provider credential",
								slog.String("request_id", reqCtx.RequestID),
								slog.String("provider", accountKey),
								slog.Any("error", tokErr),
							)
						}
						attempts = append(attempts, core.Attempt{
							Provider: accountKey,
							Model:    hop.Model,
							Status:   0,
							Elapsed:  0,
						})
						applyPenaltyModel(deps, accountKey, hop.Model, core.FailureNoRetryWithCooldown, nil, nil)
						continue
					}
				} else if prov.APIKey != "" {
					tokRes = credential.TokenResult{Value: prov.APIKey, Kind: credential.KindStatic}
				}

				var outReq *http.Request
				if prov.Transport == "cloudcode" {
					onboarder := deps.CloudCodeOnboarding
					if onboarder == nil {
						onboarder = defaultCloudCodeOnboarding
					}
					projectID, pErr := onboarder.ProjectID(ctx, tokRes.Value)
					if pErr != nil {
						if deps.Logger != nil {
							deps.Logger.Error("cloudcode onboarding failed",
								slog.String("request_id", reqCtx.RequestID),
								slog.String("provider", accountKey),
								slog.Any("error", pErr),
							)
						}
						attempts = append(attempts, core.Attempt{
							Provider: accountKey,
							Model:    hop.Model,
							Status:   0,
							Elapsed:  0,
						})
						applyPenaltyModel(deps, accountKey, hop.Model, core.FailureRetryWithCooldown, nil, nil)
						continue
					}

					exec := cloudcode.NewExecutor(nil)
					var reqErr error
					outReq, reqErr = exec.GenerateRequest(ctx, prov.BaseURL, projectID, hop.Model, tokRes.Value, rewrittenBody, parsed.Stream)
					if reqErr != nil {
						continue
					}
					if len(prov.Headers) > 0 {
						for k, v := range prov.Headers {
							if v == nil {
								outReq.Header.Del(k)
							} else if *v != "" {
								outReq.Header.Set(k, *v)
							}
						}
					}
				} else {
					var reqErr error
					outReq, reqErr = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rewrittenBody))
					if reqErr != nil {
						continue
					}
					// Never forward the inbound caller's credential upstream (D14):
					// headers come exclusively from the hop dialect's own auth shape.
					outReq.Header = hopDialect.AuthHeaders(tokRes, prov.Headers)
				}

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
				resp, err := deps.Transport.RoundTrip(outReq)
				elapsed := time.Since(start)

				if err != nil {
					if deps.Logger != nil {
						deps.Logger.Error("upstream request to provider failed: network error",
							slog.String("request_id", reqCtx.RequestID),
							slog.String("provider", accountKey),
							slog.String("model", hop.Model),
							slog.String("url", url),
							slog.Duration("elapsed", elapsed),
							slog.Any("error", err),
						)
					}
					attempts = append(attempts, core.Attempt{
						Provider: accountKey,
						Model:    hop.Model,
						Status:   0,
						Elapsed:  elapsed,
					})
					fc := core.ClassifyFailure(0)
					applyPenaltyModel(deps, accountKey, hop.Model, fc, nil, nil)
					continue
				}

				attempts = append(attempts, core.Attempt{
					Provider: accountKey,
					Model:    hop.Model,
					Status:   resp.StatusCode,
					Elapsed:  elapsed,
				})

				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					// COMMIT: first byte is about to be relayed. No further
					// hops are attempted past this point (D5).
					committed = true
					winningProvider = accountKey
					winningXlatedReq = rewrittenBody
					if parsed.Stream {
						finalUsage, respBody = relaySSE(w, resp, hopDialect.NewUsageScanner(), respTrans, state, deps.Logger)
					} else {
						if respTrans != nil {
							respBody = relayDirectTranslated(w, resp, respTrans, state)
							if state.Usage != nil {
								finalUsage = &core.Usage{
									InputTokens:         state.Usage.InputTokens,
									OutputTokens:        state.Usage.OutputTokens,
									CacheReadTokens:     state.Usage.CacheReadInputTokens,
									CacheCreationTokens: state.Usage.CacheCreationInputTokens,
								}
							}
						} else {
							respBody = relayDirect(w, resp)
							if respBody != nil {
								scanner := hopDialect.NewUsageScanner()
								scanner.Observe(respBody)
								finalUsage = scanner.Usage()
							}
						}
					}
					winningRawResp = respBody
					outcome = core.OutcomeOK

					if finalUsage != nil && deps.UsageStore != nil {
						deps.UsageStore.Record(accountKey, *finalUsage)
					}
					if prov.Selection == core.StrategyStickyRoundRobin && deps.Affinity != nil {
						deps.Affinity.Touch(accountKey)
					}

					break hopLoop
				}

				errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
				resp.Body.Close()

				fc := core.ClassifyFailure(resp.StatusCode)
				applyPenaltyModel(deps, accountKey, hop.Model, fc, resp, errBody)

				if deps.Logger != nil {
					errStr := string(errBody)
					if len(errStr) > 512 {
						errStr = errStr[:512] + "..."
					}
					deps.Logger.Error("upstream request to provider failed: non-2xx status",
						slog.String("request_id", reqCtx.RequestID),
						slog.String("provider", accountKey),
						slog.String("model", hop.Model),
						slog.Int("status", resp.StatusCode),
						slog.Duration("elapsed", elapsed),
						slog.String("error", errStr),
					)
				}

				switch fc {
				case core.FailureRetryWithCooldown, core.FailureRetryNoCooldown:
					// Before first byte: classify, penalize, try next account/hop.
					continue
				default: // FailureNoRetryWithCooldown, FailureNoRetryNoCooldown
					for k, vals := range resp.Header {
						for _, v := range vals {
							w.Header().Add(k, v)
						}
					}
					w.WriteHeader(resp.StatusCode)
					w.Write(errBody)
					if fc == core.FailureNoRetryWithCooldown {
						outcome = core.OutcomeAuthFailed
					} else {
						outcome = core.OutcomeNoRoute
					}
					committed = true
					goto record
				}
			}
		}

		if !committed && outcome == core.OutcomeChainExhausted {
			if deps.Logger != nil {
				deps.Logger.Warn("all provider hops exhausted",
					slog.String("request_id", reqCtx.RequestID),
					slog.Int("attempts", len(attempts)),
				)
			}
			inboundDialect.WriteError(w, http.StatusBadGateway, "overloaded_error",
				fmt.Sprintf("all providers exhausted (%d attempts)", len(attempts)))
		}

	record:
		latency := time.Since(handlerStart)
		// Record off the critical path (D14): the client has already
		// received its response by the time this runs.
		go recordOutcome(deps, reqCtx, r.URL.Path, parsed, body, respBody, winningXlatedReq, winningRawResp, attempts, finalUsage, outcome, latency, winningProvider)
	}
}

func recordOutcome(deps *Deps, reqCtx *RequestCtx, path string, parsed core.ParsedRequest, reqBody, respBody, xlatedReqBody, rawRespBody []byte, attempts []core.Attempt, usage *core.Usage, outcome core.Outcome, latency time.Duration, provider string) {
	rec := core.RequestRecord{
		Version:               1,
		Timestamp:             time.Now(),
		ID:                    reqCtx.RequestID,
		Key:                   reqCtx.KeyID,
		Session:               reqCtx.SessionID,
		Endpoint:              path,
		ModelReq:              parsed.Model,
		Stream:                parsed.Stream,
		Attempts:              attempts,
		Usage:                 usage,
		Outcome:               outcome,
		Latency:               latency,
		Provider:              provider,
		RequestBody:           formatResponseAsJSON(reqBody),
		ResponseBody:          formatResponseAsJSON(respBody),
		TranslatedRequestBody: formatResponseAsJSON(xlatedReqBody),
		RawResponseBody:       formatResponseAsJSON(rawRespBody),
	}

	if deps.Recorder != nil {
		deps.Recorder.Record(context.Background(), rec)
	}
}

// formatResponseAsJSON converts a response or request body (including SSE stream data) into a valid JSON string.
// If the body is already valid JSON, it is returned as a trimmed string.
// If the body is an SSE stream, data payload lines are collected into a JSON array string.
func formatResponseAsJSON(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	if !bytes.HasPrefix(trimmed, []byte("data:")) && !bytes.HasPrefix(trimmed, []byte("event:")) {
		var js interface{}
		if json.Unmarshal(trimmed, &js) == nil {
			return string(trimmed)
		}
	}

	var events []json.RawMessage
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if data, ok := cutSSEData(line); ok {
			data = bytes.TrimSpace(data)
			if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) || bytes.Equal(data, []byte("[done]")) {
				continue
			}
			if json.Valid(data) {
				events = append(events, json.RawMessage(bytes.Clone(data)))
			}
		}
	}

	if len(events) > 0 {
		if out, err := json.Marshal(events); err == nil {
			return string(out)
		}
	}

	return string(trimmed)
}

// applyPenalty cools down a provider according to its failure class.
func applyPenalty(deps *Deps, provider string, fc core.FailureClass, resp *http.Response) {
	applyPenaltyModel(deps, provider, "", fc, resp, nil)
}

// applyPenaltyModel cools down a provider/model key according to failure class and reset duration.
func applyPenaltyModel(deps *Deps, accountKey, hopModel string, fc core.FailureClass, resp *http.Response, body []byte) {
	if deps.NoPenalties {
		return
	}
	parser := deps.ResetParser
	if parser == nil {
		parser = core.NewStandardResetParser(24 * time.Hour)
	}

	parsedDuration := parser.Duration(resp, body, &fc)

	switch fc {
	case core.FailureRetryWithCooldown:
		duration := deps.Cooldown5xx
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			duration = deps.Cooldown429
		}
		if parsedDuration > 0 {
			duration = parsedDuration
		}
		deps.Health.PenalizeModel(accountKey, hopModel, duration)
	case core.FailureNoRetryWithCooldown:
		duration := 15 * time.Minute
		if parsedDuration > 0 {
			duration = parsedDuration
		}
		deps.Health.PenalizeModel(accountKey, hopModel, duration)
		if deps.Logger != nil {
			deps.Logger.Warn("provider auth error cooldown", slog.String("provider", accountKey), slog.String("model", hopModel), slog.Duration("cooldown", duration))
		} else {
			log.Printf("WARNING: provider %q model %q returned an auth error, cooled down for %v", accountKey, hopModel, duration)
		}
	case core.FailureRetryNoCooldown, core.FailureNoRetryNoCooldown:
		// No cooldown
	}
}

// relaySSE copies response headers, commits the status line, and relays the
// body line by line with a flush at each SSE event boundary (D5, D12: no
// whole-response buffering). When translator is non-nil, it translates each
// SSE chunk and drains closing frames at EOF. It observes each data line for usage (D10).
func relaySSE(w http.ResponseWriter, resp *http.Response, scanner core.UsageScanner, translator core.ResponseTranslator, state *core.StreamState, logger *slog.Logger) (*core.Usage, []byte) {
	defer resp.Body.Close()

	copyHeaders(w, resp.Header)
	if translator != nil {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)

	var captured bytes.Buffer
	br := bufio.NewReaderSize(resp.Body, 4096)

	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if data, ok := cutSSEData(trimmed); ok {
				if translator != nil {
					frames, tErr := translator.TranslateResponse(data, state)
					if tErr == nil {
						for _, frame := range frames {
							writeSSEFrame(w, frame)
							captured.Write(frame)
							captured.WriteString("\n")
						}
					}
				} else {
					scanner.Observe(data)
					w.Write(line)
					captured.Write(line)
				}
			} else if translator == nil {
				w.Write(line)
				captured.Write(line)
			}

			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			if err != io.EOF {
				// Already committed (first byte relayed): propagate and
				// log, do not retry (D5).
				if logger != nil {
					logger.Warn("mid-stream failure", slog.Any("error", err))
				} else {
					log.Printf("mid-stream failure: %v", err)
				}
			}
			break
		}
	}

	if translator != nil {
		drainFrames, tErr := translator.TranslateResponse(nil, state)
		if tErr == nil {
			for _, frame := range drainFrames {
				writeSSEFrame(w, frame)
				captured.Write(frame)
				captured.WriteString("\n")
			}
		}
		if canFlush {
			flusher.Flush()
		}
	}

	var usage *core.Usage
	if state != nil && state.Usage != nil {
		usage = &core.Usage{
			InputTokens:         state.Usage.InputTokens,
			OutputTokens:        state.Usage.OutputTokens,
			CacheReadTokens:     state.Usage.CacheReadInputTokens,
			CacheCreationTokens: state.Usage.CacheCreationInputTokens,
		}
	} else if scanner != nil {
		usage = scanner.Usage()
	}

	return usage, captured.Bytes()
}

func writeSSEFrame(w io.Writer, frame []byte) {
	var evt struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(frame, &evt) == nil && evt.Type != "" {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, frame)
	} else {
		fmt.Fprintf(w, "data: %s\n\n", frame)
	}
}

func relayDirectTranslated(w http.ResponseWriter, resp *http.Response, translator core.ResponseTranslator, state *core.StreamState) []byte {
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)

	_, _ = translator.TranslateResponse(rawBody, state)
	_, _ = translator.TranslateResponse(nil, state)

	outBody := response.NonStreamingMessageJSON(state)
	if len(outBody) == 0 {
		outBody = rawBody
	}

	copyHeaders(w, resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(outBody)))
	w.WriteHeader(resp.StatusCode)
	w.Write(outBody)
	return outBody
}

// cutSSEData extracts the payload from an SSE "data: ..." line.
func cutSSEData(line []byte) ([]byte, bool) {
	const prefix = "data: "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return nil, false
	}
	return line[len(prefix):], true
}

// relayDirect copies headers, status, and the full body for a
// non-streaming response, returning the body bytes for usage parsing.
func relayDirect(w http.ResponseWriter, resp *http.Response) []byte {
	defer resp.Body.Close()

	copyHeaders(w, resp.Header)
	w.WriteHeader(resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	w.Write(respBody)
	return respBody
}

func copyHeaders(w http.ResponseWriter, headers http.Header) {
	for k, vals := range headers {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
}

// JoinURL cleanly combines a base URL and a relative path. If the base URL ends with
// a version prefix (e.g. "/v1") and the relative path starts with the same version prefix
// (e.g. "/v1/chat/completions"), the duplicate prefix is automatically stripped.
func JoinURL(baseURL, relPath string) string {
	base := strings.TrimRight(baseURL, "/")
	rel := relPath
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	if strings.HasSuffix(base, "/v1") && (rel == "/v1" || strings.HasPrefix(rel, "/v1/")) {
		rel = rel[3:]
	}
	return base + rel
}

var (
	countersMu sync.Mutex
	counters   = make(map[string]*uint64)
)

func getProviderCounter(provider string) *uint64 {
	countersMu.Lock()
	defer countersMu.Unlock()
	ctr, ok := counters[provider]
	if !ok {
		ctr = new(uint64)
		counters[provider] = ctr
	}
	return ctr
}
