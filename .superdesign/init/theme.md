# Theme

## Compact tokens — TARGET (docs/ui.md), not the live CSS

Fonts: `system-ui, -apple-system, "Segoe UI", "Helvetica Neue", sans-serif`
Body 16px, chrome min 14px, inputs 16px, H1 1.75rem, H2 1.25rem, line-height 1.5.
Spacing: 0.25 / 0.5 / 0.75 / 1 / 1.5 / 2 / 3rem.
Radius: box 0.5rem, field 0.25rem.

### Light

- base-100 `#F4F6F6`
- base-200 `#E8ECEC`
- base-300 `#D5DCDC`
- base-content `#012A2C` (logo ink)
- primary `#0F5F5A`
- primary-content `#FFFFFF`

### Dark

- base-100 `#0F1A1A`
- base-200 `#162424`
- base-300 `#2A3A3A`
- base-content `#E8EEEE`
- primary `#4AA79F`
- primary-content `#012A2C`

No cream gradient. No rainbow UI colours. Logo on a light chip in dark mode.

## Compact tokens — CURRENT live CSS (`web/static/css/app.css`)

- `--ink: #1a1f16`
- `--bg: #f3efe6`
- `--accent: #2f5d50`
- `--line: #d9d2c5`
- `--danger: #8b2e2e`
- `--card: #fffdf8`
- `color-scheme: light` only
- Breakpoints: 479px nav type, 640px input 16px, 720px cards, 900px sidebar

## Raw current `:root`

```css
:root {
  color-scheme: light;
  --ink: #1a1f16;
  --bg: #f3efe6;
  --accent: #2f5d50;
  --line: #d9d2c5;
  --danger: #8b2e2e;
  --card: #fffdf8;
}
body {
  font-family: system-ui, -apple-system, "Segoe UI", "Helvetica Neue", sans-serif;
  background: radial-gradient(circle at top left, #fff8e8, var(--bg));
  color: var(--ink);
  line-height: 1.45;
}
```

Full file: `web/static/css/app.css` (~611 lines).
