# Layouts

Shared chrome is `web/templates/layout.html` (`layout_start` / `layout_end`). Every page wraps with those templates.

## `web/templates/layout.html`

Signed-in: `.app-chrome` with brand text “Beacon”, `.app-nav` (mobile bottom bar → sidebar from 900px), `.profile-menu`. Guest: `header.app` + Log in. Footer: `.app-version`.

Full source:

```html
{{define "layout_start"}}<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{if .Title}}{{.Title}} · {{end}}Beacon</title>
  <link rel="stylesheet" href="{{static "css/app.css"}}">
</head>
<body class="{{if and .User .User.Email}}app-shell{{end}}{{if .BodyClass}} {{.BodyClass}}{{end}}">
{{if and .User .User.Email}}
<div class="app-chrome no-print">
  <div class="brand">
    <strong><a href="/">Beacon</a></strong>
  </div>
  <nav class="app-nav" aria-label="Primary">
    <a href="/occupants">Occupants</a>
    <a href="/expenses">Expenses</a>
    <a class="nav-desktop-only" href="/operations">Operations</a>
    <a class="nav-desktop-only" href="/safeguarding">Safeguarding</a>
    <a href="/reports">Reports</a>
    <a class="nav-desktop-only" href="/dashboard">Dashboard</a>
    <a class="nav-desktop-only" href="/admin/users">Users</a>
    <a class="nav-desktop-only" href="/admin/houses">Houses</a>
    <a class="nav-desktop-only" href="/admin/audit">Audit</a>
    <details class="nav-more">
      <summary>More</summary>
      <div class="nav-more-panel">…</div>
    </details>
  </nav>
  <details class="profile-menu">
    <summary class="profile-toggle" aria-label="Account">
      <span class="profile-email">{{.User.Email}}</span>
    </summary>
    <div class="profile-panel">
      <a href="/account">Account &amp; passkeys</a>
      <form class="inline" method="post" action="/logout">
        <button class="secondary" type="submit">Log out</button>
      </form>
    </div>
  </details>
</div>
{{else}}
<header class="app no-print">
  <div class="brand"><strong><a href="/">Beacon</a></strong></div>
  <nav><a href="/login">Log in</a></nav>
</header>
{{end}}
<main class="wrap">
{{end}}

{{define "layout_end"}}
<footer class="app-footer no-print">
  <hr>
  <p class="app-version">{{appVersion}}</p>
</footer>
</main>
</body>
</html>
{{end}}
```

**Redesign target:** Rainbow Haven full logo in chrome (not the word “Beacon”). Desktop: daisyUI `drawer` + `menu`. Mobile: `navbar` + `dock`. Footer: `Beacon · {version}`.
