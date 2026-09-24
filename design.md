# Design — tinyroute dashboard

A locked design system for the tinyroute gateway dashboard (`web/`). Every page
redesign reads this file before emitting code. Do not regenerate per page —
extend or amend this file when the system needs to grow.

Supersedes the previous system (atmospheric · Workbench · "Obsidian" violet,
glassmorphism + glows). This file is the source of truth; where it conflicts
with any reference, this file wins.

## Genre

modern-minimal — the dev-tool / infra register. Flat, opaque, monochrome
charcoal with one electric cyan signal accent. An instrument panel, not a
cockpit: no glassmorphism, no glows, no gradients, no radial washes.

## Macrostructure family

All routes are **app pages** — a Workbench shell (flat sidebar + topbar +
content column). Pages vary only in component archetypes:

- **Console pages** (Overview, History): KPI row + chart / ledger table.
- **Catalog pages** (Providers, Clients): filter bar + card grid → detail view.
- **Config pages** (Combos, Keys, Settings): header + cards / tables + modals.
- **Gate page** (Login): single centered card, typography only.

## Theme — "Cyan Charcoal" (custom)

| Token | Value | Role |
|---|---|---|
| `--background` | `oklch(0.145 0.006 260)` | charcoal paper, near-zero chroma |
| `--card` | `oklch(0.175 0.007 260)` | opaque card surface |
| `--popover` | `oklch(0.19 0.007 260)` | menus, tooltips, overlays |
| `--foreground` | `oklch(0.93 0.006 260)` | primary ink |
| `--muted` | `oklch(0.205 0.007 260)` | inset fills, table head |
| `--muted-foreground` | `oklch(0.64 0.008 260)` | secondary ink |
| `--secondary` | `oklch(0.215 0.008 260)` | neutral button / selected fill |
| `--border` | `oklch(0.265 0.008 260)` | solid 1px rules — no alpha stacking |
| `--border-subtle` | `oklch(0.225 0.007 260)` | internal dividers |
| `--input` | `oklch(0.19 0.007 260)` | control surface |
| `--primary` | `oklch(0.80 0.13 195)` | electric cyan — the ONE accent |
| `--primary-foreground` | `oklch(0.17 0.03 210)` | dark ink on cyan |
| `--accent` | `oklch(0.80 0.13 195 / 12%)` | cyan wash for selected states only |
| `--accent-foreground` | `oklch(0.82 0.10 195)` | cyan-tinted text (links, values) |
| `--ring` | `oklch(0.80 0.13 195)` | focus ring |
| `--destructive` | `oklch(0.64 0.20 25)` | red — errors, destructive |
| `--success` | `oklch(0.74 0.16 155)` | green — healthy, active |
| `--warning` | `oklch(0.81 0.14 85)` | amber — caution |

Semantic colors are **token-only**. Raw Tailwind palette classes
(`text-emerald-400`, `bg-amber-500/15`, `text-rose-300`, …) are banned in
page and component code.

## Typography

- Display: **Space Grotesk** 600–700, roman only, tracking `-0.02em`.
- Body: **Plus Jakarta Sans** 400–600.
- Mono: **JetBrains Mono** 400–600.

**Mono is for machine data** — model IDs, keys, endpoints, paths, code,
numeric values, timestamps. Labels, headings, and prose are always the body
face. Uppercase micro-labels are chrome-only (nav groups, table headers) in
the sans face — never mono, never on card content titles.

Type scale (Tailwind utilities): page title `text-xl font-semibold`, card
title `text-sm font-semibold`, KPI value `text-2xl font-mono`, body `text-sm`,
dense `text-xs`.

## Spacing

Tailwind's 4-pt scale via utilities (`p-5`, `gap-4`, `space-y-6`). Page
content: `space-y-6`; cards `p-5`; grids `gap-4`/`gap-6`.

Radii: controls `rounded-md` (8px), cards `rounded-lg` (10px), pills
`rounded-full`. Tight instrument radii — nothing above 10px except overlays
(12px).

## Motion

- Easing: `cubic-bezier(0.16, 1, 0.3, 1)` named `--ease-out`.
- Transitions: 120–150ms, `color / background-color / border-color / opacity /
  transform` only.
- **Exactly one perpetual animation in the app**: the daemon-status dot
  (`animate-pulse`, 2s). Nothing else pulses, pings, or glows.
- Overlays (Modal, drawer): 150ms opacity + slight scale/slide. Backdrops are
  plain dim (`bg-black/70`) — no backdrop blur.
- `prefers-reduced-motion: reduce`: pulse becomes a steady dot; all other
  motion collapses to ≤150ms opacity.

## Microinteractions stance

- Silent success: copy buttons swap to a check icon for 2s. No toasts, no
  celebration.
- Destructive actions require a confirm dialog (`ConfirmDialog`) — never
  native `confirm()`. Errors surface in a dismissible `Banner` — never
  `alert()`.
- Hover on interactive rows/cards: border-color or background change only. No
  translate lifts, no shadows appearing, no glows.
- Focus: `focus-visible` outline, 2px `--ring`, offset 2px, shown instantly.

## CTA voice

- Primary: solid cyan fill, dark ink text, `rounded-md`, verb-first copy
  ("Create key", "Add model"). One per view.
- Secondary: neutral `--secondary` fill.
- Outline/ghost: for low-weight actions.
- Destructive: solid red **only inside confirm dialogs**; in tables and detail
  headers it is a quiet icon/ghost button that turns red on hover, never in
  the primary position.

## Copy tone

Plain operator language. Name the thing. Banished: "Architect", "Deploy",
"Mesh", "Matrix", "Ledger", "Velocity", "hops", "Reliability" (→ "Success
rate"), "Total Volume" (→ "Requests"). Sentences over fragments where a
fragment would be cute.

## What pages MUST share

- Wordmark ("tinyroute" + route glyph), sidebar/topbar shell, nav grouping.
- Cyan accent and its placement rules: primary CTA, active state, links,
  focus, data highlights. ≤ 5% of any viewport.
- The type stack and mono-for-data discipline.
- Button, Input, Select, Modal, Banner, ConfirmDialog, StatusBadge — no
  page-local re-implementations (KPIs go through `KpiCard`).

## What pages MAY differ on

- Component archetypes within the page family (grid vs table vs wizard).
- Column counts and card arrangement.
- Empty-state copy.

## Per-page allowances

- App pages: no enrichment, no reveals, no hero imagery — function carries.
- Login: typography only.

## Exports

See `web/src/styles/tokens.css` (canonical values) and the Tailwind v4
`@theme` mapping in `web/src/styles/index.css`. Variable names follow the
shadcn/ui convention (`--background`, `--card`, `--primary`, …) so the same
tokens drop into any shadcn-style project:

```css
:root {
  --background: oklch(0.145 0.006 260);
  --foreground: oklch(0.93 0.006 260);
  --card: oklch(0.175 0.007 260);
  --primary: oklch(0.80 0.13 195);
  --primary-foreground: oklch(0.17 0.03 210);
  --muted: oklch(0.205 0.007 260);
  --muted-foreground: oklch(0.64 0.008 260);
  --border: oklch(0.265 0.008 260);
  --ring: oklch(0.80 0.13 195);
  --radius: 0.5rem;
}
```
