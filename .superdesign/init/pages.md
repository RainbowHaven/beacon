# Page dependency trees

Go templates include `layout.html` plus the page file. CSS is always `web/static/css/app.css`. JS only where listed.

```
login
  web/templates/login.html
  web/templates/layout.html
  web/static/css/app.css
  web/static/js/webauthn.js

occupants
  web/templates/occupants.html
  web/templates/layout.html
  web/static/css/app.css

expense_new
  web/templates/expense_new.html
  web/templates/expense_fields.html
  web/templates/layout.html
  web/static/css/app.css
  web/static/js/receipt-resize.js

reports
  web/templates/reports.html
  web/templates/report_sections.html
  web/templates/layout.html
  web/static/css/app.css

home
  web/templates/home.html
  web/templates/layout.html
  web/static/css/app.css

expenses
  web/templates/expenses.html
  web/templates/layout.html
  web/static/css/app.css

occupant_edit
  web/templates/occupant_edit.html
  web/templates/layout.html
  web/static/css/app.css

account
  web/templates/account.html
  web/templates/layout.html
  web/static/css/app.css
  web/static/js/webauthn.js
```
