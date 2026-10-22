package httpapi

import (
	"net/http"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
)

// handleListSkills answers with the skills installed on this machine. The list
// is read from ~/.calabrass/skills every time rather than cached: a person can
// drop a folder there and reload the page, which is the whole point of skills
// being files.
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := agent.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}
