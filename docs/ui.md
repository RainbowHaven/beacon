# Beacon UI

Source of truth for visual design and HTML in this repo. Agents and humans follow this file, not the official daisyUI “use Tailwind utilities in HTML” default.

Stack stays [AGENTS.md](../AGENTS.md): server-rendered Go templates, no React, no HTMX, minimal vanilla JS. Production templates follow this document.

## Brand

Rainbow Haven is the NGO. Beacon is the product.

| Surface | What to show |
|---|---|
| App chrome (logged in and guest) | Full lockup: rainbow, house, wordmark **RAINBOW HAVEN**. Link to `/`. `alt="Rainbow Haven"`. |
| Document `<title>` | `{page} · Beacon` |
| Footer | `Beacon · {version}` — do not repeat the wordmark if it is already in the header |
| Favicon / app icon | Mark only (rainbow + house), [web/static/img/favicon.svg](../web/static/img/favicon.svg) |
| Print report | Product name Beacon only — no Rainbow Haven lockup in the document |

**Files**

- Full lockup: [web/static/img/rainbow-haven.svg](../web/static/img/rainbow-haven.svg)
- Mark: [web/static/img/rainbow-haven-mark.svg](../web/static/img/rainbow-haven-mark.svg)
- Serve with `{{static}}` (cache buster). Do not rasterize for the page.

**Lockup rules**

- Minimum logo height in the header: 40px. Clear space around it at least 8px.
- One primary action per view. The logo is identity, not decoration.
- **Do not** use the six rainbow stripe colours on buttons, alerts, charts, or backgrounds. The rainbow lives in the logo.
- Ink sampled from the wordmark is `#012A2C`. Lift it for `primary` buttons so white label text stays WCAG AA.

**Dark mode**

The teal wordmark (`#012A2C`) disappears on a dark background. Sit the SVG on a light chip (`.brand-mark`, `brand-chip` `#FFFFFF` in both themes, short radius). Do not `filter: invert` — that would wreck the rainbow. Do not use `base-100`: in dark mode that is `#161616`.

```html
<a class="brand-mark" href="/">
  <img src="{{static "img/rainbow-haven.svg"}}" alt="Rainbow Haven" width="160" height="144">
</a>
```

## Principles

Distilled from [Refactoring UI](https://www.refactoringui.com/) for this app:

1. **Hierarchy uses one lever at a time.** Size *or* weight *or* colour, not all three on the same string.
2. **Start with too much space**, then tighten. Use the spacing scale; no one-off `margin: 13px`.
3. **One primary action** per view (`btn btn-primary`). Everything else is `btn-outline` / `btn-ghost`.
4. **Empty states are designed.** A sentence plus the primary action, not a blank table.
5. **No decoration without a job.** No craft-paper gradients, no rainbow UI, no drop shadows for their own sake.
6. **Type is for tired eyes.** Body 16px. Chrome not under 14px. Inputs 16px (iOS zoom).
7. **Contrast is a requirement.** WCAG AA for text and UI chrome. Prefer semantic daisyUI colours (`primary`, `base-content`, `error`) over grey-on-tint.
8. **Group by proximity and cards**, not by extra borders and extra colours.

## HTML contract

Allowed in templates:

- daisyUI **component** classes and their documented parts, colours, sizes, and daisyUI modifiers (`btn`, `btn-primary`, `navbar`, `lg:drawer-open`, `alert-error`, …)
- Beacon **named layout** classes listed below

Forbidden in templates:

- Tailwind utility soup (`flex`, `p-4`, `text-sm`, `md:w-1/2`, `gap-2`, `overflow-x-auto`, …)
- Bootstrap-style helpers (`d-flex`, `mb-3`)
- Inline `style="…"`
- Raw hex / rgb colours
- New one-off class names invented in a single template

Layout that daisyUI does not cover → add a named class in CSS (`web/static/css/` / future `beacon.css`), never a utility string in HTML.

The official daisyUI skill says “if daisyUI cannot do it, use Tailwind utilities.” **That rule is overridden here.**

## Tokens

**Type:** Fira Sans (Google Fonts) for UI, including buttons even when they sit inside a table. [Ioskeley Mono](https://github.com/ahatem/IoskeleyMono) v2.1.0 (OFL, Iosevka build inspired by Berkeley Mono) for **table cells only** — Regular/Medium/Bold WOFF2 under [web/static/fonts/ioskeley/](../web/static/fonts/ioskeley/). Not an official Berkeley Mono. Report prose, headings, facts and stats stay Fira Sans.

` "Fira Sans", system-ui, sans-serif`
` "Ioskeley Mono", ui-monospace, monospace` — `.table` cells (`tabular-nums`, ligatures off, slashed zero). Not inputs, labels, or `.btn`.

### Type

| Role | Size | Notes |
|---|---|---|
| Body | `1rem` (16px) | Default |
| Small / chrome | `0.875rem` (14px) | Minimum. Never smaller. |
| H1 | `1.75rem` | Page title. Weight 500 (Fira Sans Medium — 700 is too heavy) |
| H2 | `1.25rem` | Section. Weight 500 |
| Numeric | `tabular-nums` | Money, dates, headcount |

Line height 1.5 for body, 1.25 for headings.

### Spacing

`0.25 / 0.5 / 0.75 / 1 / 1.5 / 2 / 3rem`. Default stack gap is `1rem`.

### Radii and size (daisyUI theme)

- `--radius-box: 0.5rem` (cards, alerts)
- `--radius-field: 0.25rem` (buttons, inputs)
- `--size-field: 0.28125rem` and `--size-selector: 0.28125rem` (slightly larger than daisyUI default — not “tiny mobile”)
- `--border: 1px`
- `--depth: 0` (flat; no toy 3D)
- `--noise: 0`

### Colour (daisyUI semantic names)

Do not put these hexes in HTML. They belong in the theme CSS.

**Light (`data-theme="light"`, default)**

| Token | Value | Role |
|---|---|---|
| `base-100` | `#F5F5F5` | Page (neutral grey, no teal wash) |
| `base-200` | `#ECECEC` | Sidebar, chips |
| `base-300` | `#D4D4D4` | Borders |
| `base-content` | `#1A1A1A` | UI text |
| `table-content` | `#111111` | `th`/`td` and report prose. Darker than `base-content` so tables match the report. |
| `brand-chip` | `#FFFFFF` | Always-light plate behind the Rainbow Haven lockup |
| `primary` | `#0F5F5A` | One accent; primary button **and** selected menu |
| `primary-content` | `#FFFFFF` | On primary |
| `neutral` | `#2A2A2A` | Unused for nav highlight |
| `info` / `success` / `warning` / `error` | daisyUI defaults unless contrast fails AA | Status only |

No cream, no radial warm gradient. Override daisyUI `.menu { --menu-active-bg: var(--color-primary) }` — daisyUI’s default `menu-active` uses `neutral`, which is a different colour from `btn-primary`. That mismatch is not intentional.

daisyUI `.label` is 60% opacity; Beacon labels use full `base-content`. Tables and `.report-doc` use `table-content`. DaisyUI’s `thead` uses `color-mix(… 60%, transparent)` at 0.875rem/600 — Beacon overrides `color`, size and weight on `.table th` / `.table td` with `!important` so dashboard and reports match.

**Dark (`data-theme="dark"`)**

| Token | Value |
|---|---|
| `base-100` | `#161616` |
| `base-200` | `#1F1F1F` |
| `base-300` | `#333333` |
| `base-content` | `#F3F3F3` |
| `table-content` | `#FAFAFA` |
| `brand-chip` | `#FFFFFF` |
| `primary` | `#4A9BA7` |
| `primary-content` | `#0A0A0A` |

Dark surfaces are true greys. Do not mix green into `base-*`.

Theme on `<html data-theme="light|dark">`. Persist with a **cookie** so the first HTML response matches (no flash). `prefers-color-scheme` is the default only when no cookie exists. Toggle with a `.theme-item` button plus a tiny script — no new JS framework.

## Named layout classes

Implement these in Beacon CSS (not as Tailwind in HTML):

| Class | Job |
|---|---|
| `.page` | Main column: **width 100%**, max ~64rem, left of the main column (not shrink-wrapped/centered). Signed-in: no extra left padding — title lines up with the topbar rule. Guest pages (no sidebar): `1.5rem` inline padding both sides (desktop `2rem`). End padding `1.5rem` (desktop `2rem`), safe-area aware |
| `.stack` | Vertical rhythm, gap `1rem`. Forms: max-width `28rem` unless the page is a wide table. Fields stretch; `.btn` keeps daisyUI’s default width (not `btn-block`). Overrides daisyUI’s overlapping `.stack` (that component fades child 2+ to 70% opacity and stacks them as avatars — Beacon does not use it; reset with `!important`). |
| `.auth-product` | Product name under the auth logo. |
| `.cluster` | Horizontal group, wrap, gap `0.75rem` (page actions, filters) |
| `.filter-bar` | Filter row: labeled fields, `align-items: flex-end`. Submit on change (no Filter/Show button; `<noscript>` fallback) |
| `.field` | Caption above control in a filter bar. Body size (1rem), not a tiny caption |
| `.menu-icon` / `.menu-label` | Sidebar icon + text; label hides when the sidebar is collapsed |
| `.sidebar-toggle` | Collapse/expand the desktop sidebar (`data-sidebar="collapsed"` on `<html>`) |
| `.table-scroll` | Horizontal scroll wrapper for tables (replaces `overflow-x-auto`). Space above the table in a report section is `1rem`. |
| `.brand-mark` | Light chip behind the logo (required in dark mode, optional in light) |
| `.report-doc` | Report body: Fira Sans. Nested `.table` uses Ioskeley Mono |
| `.theme-item` | Theme control: moon/sun icons; label is the opposite of the current mode |
| `.receipt-box` | Wider `modal-box` for a receipt image or PDF |
| `.receipt-preview` | Receipt `<img>`: contain, max height `70dvh` |
| `.receipt-frame` | Receipt PDF `<iframe>`: same max height |

## Component catalogue

Read the matching file under `.agents/skills/daisyui/components/` before using a component.

| Beacon pattern | daisyUI | Notes |
|---|---|---|
| Primary / secondary / danger button | `btn btn-primary` / `btn btn-outline` / `btn btn-error` | Default size. Avoid `btn-xs`. Always Fira Sans, including table actions. |
| Flash / form error | `alert` + `alert-success` / `alert-error` / `alert-info`, `role="alert"` | One slot in the layout, not copy-paste per page |
| Text / email / date | `input` | 16px effective size |
| Select | `select` | Keep `class="select"` so the chevron shows. Do not set `background` on `.select` in Beacon CSS. |
| Textarea | `textarea` | |
| File | `file-input` | Receipts |
| Label + hint | `fieldset` + `fieldset-legend` + `label` | |
| Data table (desktop) | `table table-zebra` inside `.table-scroll` | |
| Record list (mobile / cards) | `list` / `card card-border` | Occupants/expenses on small screens |
| Headcount | `stats` | Home and occupants |
| Status (review, current) | `badge` | `badge-outline` or semantic colour |
| Guest header | `navbar` | Logo start, Log in end |
| Signed-in desktop | `drawer` + `lg:drawer-open` + `menu` with icons | Logo at top of sidebar. Collapse to icons; persist. |
| Signed-in mobile | `navbar` (logo + account) + `dock` | Occupants, Expenses, Reports, More. Min 44px targets, `viewport-fit=cover` |
| Account / more | `dropdown` with `<details>` | No extra JS |
| Theme toggle | `.theme-item` button (moon/sun icons) | Desktop: sidebar **footer**, above Account. The footer (Dark mode, Account, Log out) sits at the bottom of the sidebar. Mobile: topbar. No checkbox. Cookie persist. Visible label and `title` are the **next** mode (Dark mode in light, Light mode in dark) |
| Destructive confirm | native `confirm()` for now, or `modal` if we add one | Document 37 uses `dialog.modal` |
| Receipt viewer | `dialog.modal` | Expense review and the expenses list. Native ESC and backdrop close. The `/receipt` URL stays the image or PDF (no-JS fallback). |

Unused daisyUI (hero, chat, rating, carousel, mockups, aura, …): do not add.

## Page recipes

**Auth (login, invite)**  
Centered `.stack` (max 28rem). Full logo **centered** above the product name **Beacon**, then the heading. One `btn-primary`. Errors as `alert-error`. Only the login passkey help uses daisyUI `link link-primary` — do not restyle nav or other `li a`.

**List (occupants, expenses, admin users)**  
`.page-header` with title + primary action. Optional filters in `.filter-bar` (labeled `.field` + daisyUI `select`; change submits). Desktop: table. Narrow: `list` / `card`. Empty: muted sentence + the same primary action.

**Form (occupant, expense, operations)**  
`.stack` of `fieldset`s. Labels always visible (no placeholder-only), start-aligned. One `btn-primary` submit. Cancel is `btn btn-outline`, in a `.cluster` with Save. Receipts use daisyUI `file-input` inside a `fieldset` (same as Log expense) so daisyUI overlap-stack `height: 100%` does not stretch the control. Field height is `calc(var(--size-field) * 10)`. Do not set `height` on `.btn` / `.input` / `.select`.

**Report**  
Screen: `.filter-bar` (named month `<select>`, house `<select class="select">` so the chevron is visible) plus toolbar `.no-print` (print, CSV), then a blank daisyUI `divider`, then `.report-doc`. Do not use `<input type="month">` — native month fields show `YYYY-MM` without a clear picker. Summary `stats` in one horizontal row (`width: fit-content`). Paper: `@media print` — hide chrome and dock, title **Monthly report · Beacon**, no Rainbow Haven lockup. Report prose is Fira Sans; only `.table` is Ioskeley Mono. daisyUI does not replace print CSS.

**Dashboard** (RHC / RHL admin)  
First item in the sidebar (and first in the phone More panel). Confirmation status for the last month that has ended — one row per safe house. `.page-header` plus a short stack of wrapping copy, then `.table-scroll`. Headers do not wrap mid-word; the table scrolls horizontally.

**Admin houses**  
`.page-header`. Edit rows are `.cluster.cluster-end` of `.field` + daisyUI `input`/`select` (Fira Sans, bottoms aligned). Do not wrap those forms in `.table` or `.filter-bar` (filter bars autosubmit on change).

**Safeguarding**  
`.page-header` with Record concern + Download Document 37. Explanations live in a `dialog.modal`; the page does not show the long copy. House filter uses the same `.filter-bar` / `.field` / `select` as Reports.

**Operations**  
Same list recipe: `.page-header`, status as `.cluster` of buttons, house filter as `.filter-bar` (no Filter button).

**Admin**  
Same list/form recipes. Role-gated nav stays server-side.

## CSS delivery

Vendored daisyUI is [web/static/css/daisyui.css](../web/static/css/daisyui.css). Beacon theme and named layout live in [web/static/css/beacon.css](../web/static/css/beacon.css). Both are hashed via `{{static}}`, with `Cache-Control: immutable` when `v=` is set. HTML is `no-store`. **Keep that model.** No CSS CDN for daisyUI.

## New pattern

1. Read the daisyUI component skill.
2. If it fits, use those classes only.
3. If it does not, add a named class in CSS and document it in this file.
4. Never fix it with utilities or inline styles in the template.

## PR checklist

- [ ] Body ≥ 16px, chrome ≥ 14px, inputs ≥ 16px
- [ ] Text/UI contrast AA in light **and** dark
- [ ] No Tailwind/Bootstrap utilities or inline styles in HTML
- [ ] One `btn-primary` per view
- [ ] Flash/errors use `alert` with `role="alert"`
- [ ] Logo is the Rainbow Haven lockup; no rainbow used as UI colour
- [ ] Dark mode: logo remains readable (chip or light asset)
- [ ] Print: reports still hide app chrome
- [ ] Static assets use `{{static}}` (cache buster)
