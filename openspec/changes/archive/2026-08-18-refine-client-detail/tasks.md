# Tasks: Refine the Client Detail Page

## 1. Component script mounting (prerequisite)

- [x] 1.1 Add `internal/dashboard/components/dialog/js.go`: embed `dialog.js` via `//go:embed` and expose it (bytes or `fs.FS`) for serving
- [x] 1.2 Register `GET /dashboard/assets/dialog.js` in `internal/dashboard/handler.go` next to the existing assets handler
- [x] 1.3 Add `<script defer src="/dashboard/assets/dialog.js"></script>` to `internal/dashboard/view_layout.templ` beside the Alpine CDN tag
- [x] 1.4 Add/extend a handler test asserting the route serves the embedded script with 200

## 2. Header and cleanup

- [x] 2.1 In `view_client_detail.templ`: delete the `ID: {ClientID} • Dialect: {Dialect}` subtitle line
- [x] 2.2 Add `@clientLogo(data.ClientID, data.ClientName)` to the header between the back-arrow and the title block
- [x] 2.3 Remove the helper paragraph under "Endpoint & Key Configuration"
- [x] 2.4 Remove the helper paragraph under "Model Slots"
- [x] 2.5 Delete the entire "Experimental Settings" block (deferred toggles)

## 3. Model picker rebuild

- [x] 3.1 Wrap each slot's trigger button + dialog content in `@dialog.Dialog(dialog.Props{ID: "picker_" + slot.ID})`
- [x] 3.2 Replace `@click="$refs.picker_x.showModal()"` on the slot button with `{ dialog.Trigger(ctx)... }` spread
- [x] 3.3 Replace the native `<dialog>` with `@dialog.Content(...)` (dark-slate `Class` override, scrollable option area) containing `Header`/`Title`
- [x] 3.4 Render every option row — `(None / Default)` and each model — as the same button component with a static `class`; selected state only via a pure-expression `:class` accent + `x-show` check icon
- [x] 3.5 Give each option row `{ dialog.Close(ctx)... }` alongside its Alpine `@click` slot update (and `oneM` reset for None/Default)
- [x] 3.6 Remove the now-unused `.model-picker-dialog` rules from `internal/dashboard/assets/input.css`
- [x] 3.7 Update/extend `clients_test.go` render tests: uniform option rows, no ID/dialect subtitle, no Experimental Settings text, dialog markup present

## 4. Regenerate and verify

- [x] 4.1 Run `templ generate` and `gofmt -w .`
- [x] 4.2 Rebuild CSS: `npx -y tailwindcss@3.4.19 -i internal/dashboard/assets/input.css -o internal/dashboard/assets/styles.css`; confirm `px-5` and `max-h-[65vh]` now present
- [x] 4.3 `go build ./...` and `go test ./...` pass
- [x] 4.4 Manual check via `go run . serve` → `/dashboard/clients/claude`: logo in header; no subtitle/helper paragraphs/Experimental block; Apply Changes padded; picker opens as modal with uniform styled rows, selected accent, check icon, select-closes; Esc/backdrop dismiss; 1M context checkbox and Apply preview flow intact