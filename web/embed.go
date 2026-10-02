package web

import "embed"

//go:embed templates/*.html
var Templates embed.FS

//go:embed static
var Static embed.FS

// Docs holds forms served only to signed-in users, never under /static/.
//
//go:embed docs/*
var Docs embed.FS

// Blank Document 37 (Safeguarding and Incident Form). Update the version when
// the file is replaced.
const (
	Document37Path     = "docs/37_Safeguarding_and_Incident_Form.docx"
	Document37Filename = "37_Safeguarding_and_Incident_Form.docx"
	Document37Version  = "12 September 2026"
)
