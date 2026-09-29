package server

import (
	"encoding/json"
	"net/http"

	"github.com/magiconair/beacon/internal/domain"
)

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	u, _ := s.currentUser(r)
	events, err := s.store.ListAuditEvents(r.Context(), 200)
	if err != nil {
		http.Error(w, "failed to load audit log", http.StatusInternalServerError)
		return
	}
	type row struct {
		domain.AuditEvent
		When string
		Meta string
	}
	rows := make([]row, 0, len(events))
	for _, e := range events {
		meta := ""
		if len(e.Meta) > 0 {
			b, err := json.Marshal(e.Meta)
			if err == nil {
				meta = string(b)
			}
		}
		rows = append(rows, row{
			AuditEvent: e,
			When:       e.CreatedAt.UTC().Format("01/02/2006 15:04:05 UTC"),
			Meta:       meta,
		})
	}
	s.render(w, "audit.html", map[string]any{
		"Title":  "Audit log",
		"User":   &u,
		"Events": rows,
	})
}
