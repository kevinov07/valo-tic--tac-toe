package handler

import (
	"net/http"
	"strings"

	"valo-tic-tac-toe-backend/internal/repository"
)

type PlayerHandler struct {
	repo *repository.PlayerRepository
}

func NewPlayerHandler(repo *repository.PlayerRepository) *PlayerHandler {
	return &PlayerHandler{repo: repo}
}

type playerSearchResponse struct {
	Alias           string `json:"alias"`
	CurrentTeamName string `json:"current_team_name"`
	Role            string `json:"role,omitempty"`
	CountryCode     string `json:"country_code,omitempty"`
}

func (h *PlayerHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []playerSearchResponse{})
		return
	}

	players, err := h.repo.SearchPlayers(q, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := make([]playerSearchResponse, len(players))
	for i, p := range players {
		item := playerSearchResponse{
			Alias:           p.Alias,
			CurrentTeamName: p.CurrentTeamName,
			CountryCode:     p.CountryCode,
		}
		if p.Role != nil {
			item.Role = string(*p.Role)
		}
		resp[i] = item
	}

	writeJSON(w, http.StatusOK, resp)
}
