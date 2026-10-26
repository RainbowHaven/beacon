# Routes

Go mux in `internal/server/server.go`. Layout: `web/templates/layout.html` on every HTML page.

| Path | Template | Summary |
|---|---|---|
| `GET /` | `home.html` | Headcount or login CTA |
| `GET /login` | `login.html` | Passkey form |
| `GET /invite/{token}` | `invite.html` | Passkey enrollment |
| `GET /help/passkeys` | `help_passkeys.html` | Help |
| `GET /account` | `account.html` | Passkeys |
| `GET /occupants` | `occupants.html` | List + headcount |
| `GET /occupants/new` | `occupant_new.html` | Form |
| `GET /occupants/{id}/edit` | `occupant_edit.html` | Form |
| `GET /expenses` | `expenses.html` | List + filter |
| `GET /expenses/new` | `expense_new.html` | Form + receipt |
| `GET /expenses/{id}/edit` | `expense_edit.html` | Form + review |
| `GET /operations` | `operational_issues.html` | Cards/list |
| `GET /operations/new` | `operational_issue_form.html` | Form |
| `GET /safeguarding` | `safeguarding.html` | Table |
| `GET /reports` | `reports.html` + `report_sections.html` | Monthly report |
| `GET /dashboard` | `dashboard.html` | Admin table |
| `GET /admin/users` | `admin_users.html` | Users |
| `GET /admin/houses` | `admin_houses.html` | Houses |
| `GET /admin/audit` | `audit.html` | Audit table |
