# Beacon UI

Source of truth for visual design and HTML in this repo. Agents and humans follow this file, not the official daisyUI “use Tailwind utilities in HTML” default.

Stack stays [AGENTS.md](../AGENTS.md): server-rendered Go templates, no React, no HTMX, minimal vanilla JS.

## Brand

Rainbow Haven is the NGO. Beacon is the product.

| Surface | What to show |
|---|---|
| App chrome (logged in and guest) | Full lockup: rainbow, house, wordmark **RAINBOW HAVEN**. Link to `/`. `alt="Rainbow Haven"`. |
| Document `<title>` | `{page} · Beacon` |
| Footer | `Beacon · {version}` — do not repeat the wordmark if it is already in the header |
| Favicon / app icon | Mark only (rainbow + house), [web/static/img/favicon.svg](../web/static/img/favicon.svg) |
| Print report | Small full lockup plus the product name Beacon |

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

The teal wordmark (`#012A2C`) disappears on a dark background. Sit the SVG on a light chip (`.brand-mark`, `base-100` fill, short radius). Do not `filter: invert` — that would wreck the rainbow.

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

System UI stack only (no webfont download):

`system-ui, -apple-system, "Segoe UI", "Helvetica Neue", sans-serif`

### Type

| Role | Size | Notes |
|---|---|---|
| Body | `1rem` (16px) | Default |
| Small / chrome | `0.875rem` (14px) | Minimum. Never smaller. |
| H1 | `1.75rem` | Page title |
| H2 | `1.25rem` | Section |
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
| `base-100` | `#F4F6F6` | Page |
| `base-200` | `#E8ECEC` | Sidebar, chips |
| `base-300` | `#D5DCDC` | Borders |
| `base-content` | `#012A2C` | Text (logo ink) |
| `primary` | `#0F5F5A` | One accent; primary button |
| `primary-content` | `#FFFFFF` | On primary |
| `neutral` | `#3D4A4A` | Muted chrome |
| `info` / `success` / `warning` / `error` | daisyUI defaults unless contrast fails AA | Status only |

No cream, no radial warm gradient.

**Dark (`data-theme="dark"`)**

| Token | Value |
|---|---|
| `base-100` | `#0F1A1A` |
| `base-200` | `#162424` |
| `base-300` | `#2A3A3A` |
| `base-content` | `#E8EEEE` |
| `primary` | `#4AA79F` |
| `primary-content` | `#012A2C` |

Theme on `<html data-theme="light|dark">`. Persist with a **cookie** so the first HTML response matches (no flash). `prefers-color-scheme` is the default only when no cookie exists. Toggle with daisyUI `theme-controller` plus a tiny script or form POST — no new JS framework.

## Named layout classes

Implement these in Beacon CSS (not as Tailwind in HTML):

| Class | Job |
|---|---|
| `.page` | Main column: max width ~64rem, horizontal padding `1rem`, safe-area aware |
| `.stack` | Vertical rhythm, gap `1rem`. Forms: max-width `28rem` unless the page is a wide table |
| `.cluster` | Horizontal group, wrap, gap `0.75rem` (page actions, filters) |
| `.page-header` | Title row: heading + `.cluster` of actions |
| `.table-scroll` | Horizontal scroll wrapper for tables (replaces `overflow-x-auto`) |
| `.brand-mark` | Light chip behind the logo (required in dark mode, optional in light) |
| `.no-print` / `.print-only` | Report toolbar vs paper |

## Component catalogue

Read the matching file under `.agents/skills/daisyui/components/` before using a component.

| Beacon pattern | daisyUI | Notes |
|---|---|---|
| Primary / secondary / danger button | `btn btn-primary` / `btn btn-outline` / `btn btn-error` | Default size. Avoid `btn-xs`. |
| Flash / form error | `alert` + `alert-success` / `alert-error` / `alert-info`, `role="alert"` | One slot in the layout, not copy-paste per page |
| Text / email / date | `input` | 16px effective size |
| Select | `select` | |
| Textarea | `textarea` | |
| File | `file-input` | Receipts |
| Label + hint | `fieldset` + `fieldset-legend` + `label` | |
| Data table (desktop) | `table table-zebra` inside `.table-scroll` | |
| Record list (mobile / cards) | `list` / `card card-border` | Occupants/expenses on small screens |
| Headcount | `stats` | Home and occupants |
| Status (review, current) | `badge` | `badge-outline` or semantic colour |
| Guest header | `navbar` | Logo start, Log in end |
| Signed-in desktop | `drawer` + `lg:drawer-open` + `menu` | Logo at top of sidebar |
| Signed-in mobile | `navbar` (logo + account) + `dock` | Occupants, Expenses, Reports, More. Min 44px targets, `viewport-fit=cover` |
| Account / more | `dropdown` with `<details>` | No extra JS |
| Theme toggle | `theme-controller` + `swap` or a labeled checkbox | Cookie persist |
| Destructive confirm | native `confirm()` for now, or `modal` if we add one | |

Unused daisyUI (hero, chat, rating, carousel, mockups, aura, …): do not add.

## Page recipes

**Auth (login, invite)**  
Centered `.stack` (max 28rem). Full logo above the heading. One `btn-primary`. Errors as `alert-error`.

**List (occupants, expenses, admin users)**  
`.page-header` with title + primary action. Optional filters in `.cluster`. Desktop: table. Narrow: `list` / `card`. Empty: muted sentence + the same primary action.

**Form (occupant, expense, operations)**  
`.stack` of `fieldset`s. Labels always visible (no placeholder-only). One `btn-primary` submit. Cancel is a link or `btn-ghost`.

**Report**  
Screen: toolbar `.no-print` (month picker, print, CSV). Paper: `@media print` — hide chrome and dock, show small logo + “Beacon” + report body. daisyUI does not replace print CSS.

**Admin**  
Same list/form recipes. Role-gated nav stays server-side.

## CSS delivery

Today: [web/static/css/app.css](../web/static/css/app.css) embedded, hashed query via `{{static}}`, `Cache-Control: immutable` when `v=` is set. HTML is `no-store`. **Keep that model.**

Next implementation phase (not this document’s job to ship): vendor daisyUI CSS into `web/static/css/`, add Beacon theme + named layout in the same or a companion file. No CDN. Optional later Tailwind CLI purge if the vendored file is too large for first load.

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
