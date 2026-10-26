# Shared UI primitives

Beacon is server-rendered Go HTML, not React. There is no `src/components/` tree. Visual primitives live as CSS classes in `web/static/css/app.css` and HTML in `web/templates/`.

Target language (not yet applied in templates): daisyUI component classes, documented in `docs/ui.md`.

## Current primitives (`web/static/css/app.css`)

### Flash / notice

```css
.flash { padding: 0.75rem 1rem; border-radius: 0.4rem; background: color-mix(in srgb, var(--accent) 12%, var(--card)); }
.flash.error { background: color-mix(in srgb, var(--danger) 12%, var(--card)); color: var(--danger); }
```

### Buttons

Templates use `<button>`, `<a class="btn">`, `.secondary`, `.danger`.

### Forms

`.stack` vertical form, `form.stack { max-width: 28rem }`. Many templates still use inline `style="max-width:…"`.

### Tables

`.table-scroll` + `table`. `.actions` for row buttons.

### Record cards

`.record-list` / `.record-card` — used on some list pages at narrow widths.

Target replacements (see `docs/ui.md`): `alert`, `btn btn-primary`, `input`, `select`, `table`, `card`, `list`, `badge`, `stats`.
