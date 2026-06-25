package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"valo-tic-tac-toe-backend/internal/model"
	"valo-tic-tac-toe-backend/internal/service"
)

type GameHandler struct {
	engine *service.GameEngine
	mu     sync.RWMutex
	games  map[string]*service.Board
}

func NewGameHandler(engine *service.GameEngine) *GameHandler {
	return &GameHandler{
		engine: engine,
		games:  make(map[string]*service.Board),
	}
}

type categoryResponse struct {
	Kind  model.CategoryKind `json:"kind"`
	Value string             `json:"value"`
	Label string             `json:"label"`
}

type cellResponse struct {
	Answered    bool   `json:"answered"`
	PlayerAlias string `json:"player_alias,omitempty"`
	TeamName    string `json:"team_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

type boardResponse struct {
	ID    string               `json:"id"`
	Rows  []categoryResponse   `json:"rows"`
	Cols  []categoryResponse   `json:"cols"`
	Cells [][]cellResponse     `json:"cells"`
}

type guessRequest struct {
	Row         int    `json:"row"`
	Col         int    `json:"col"`
	PlayerAlias string `json:"player_alias"`
}

	type guessResponse struct {
		Correct bool `json:"correct"`
		Player  *struct {
			Alias           string `json:"alias"`
			CurrentTeamName string `json:"current_team_name"`
			AvatarURL       string `json:"avatar_url"`
		} `json:"player,omitempty"`
	}

func (h *GameHandler) CreateGame(w http.ResponseWriter, r *http.Request) {
	id := uuid.NewString()

	board, err := h.engine.GenerateBoard(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	h.mu.Lock()
	h.games[id] = board
	h.mu.Unlock()

	writeJSON(w, http.StatusCreated, toBoardResponse(board))
}

func (h *GameHandler) GetGame(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "gameID")

	board, ok := h.getGame(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("partida no encontrada"))
		return
	}

	writeJSON(w, http.StatusOK, toBoardResponse(board))
}

func (h *GameHandler) Guess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "gameID")

	board, ok := h.getGame(id)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("partida no encontrada"))
		return
	}

	var req guessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("cuerpo JSON inválido"))
		return
	}

	correct, player, err := h.engine.CheckGuess(board, req.Row, req.Col, req.PlayerAlias)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCell):
			writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, service.ErrCellAlreadyAnswered):
			writeError(w, http.StatusConflict, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	resp := guessResponse{Correct: correct}
	if correct && player != nil {
		resp.Player = &struct {
			Alias           string `json:"alias"`
			CurrentTeamName string `json:"current_team_name"`
			AvatarURL       string `json:"avatar_url"`
		}{
			Alias:           player.Alias,
			CurrentTeamName: player.CurrentTeamName,
			AvatarURL:       player.AvatarURL,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *GameHandler) getGame(id string) (*service.Board, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	board, ok := h.games[id]
	return board, ok
}

func toBoardResponse(b *service.Board) boardResponse {
	resp := boardResponse{
		ID:    b.ID,
		Rows:  make([]categoryResponse, len(b.Rows)),
		Cols:  make([]categoryResponse, len(b.Cols)),
		Cells: make([][]cellResponse, len(b.Cells)),
	}

	for i, row := range b.Rows {
		resp.Rows[i] = categoryResponse{
			Kind:  row.Kind,
			Value: row.Value,
			Label: row.Label,
		}
	}

	for i, col := range b.Cols {
		resp.Cols[i] = categoryResponse{
			Kind:  col.Kind,
			Value: col.Value,
			Label: col.Label,
		}
	}

	for i, row := range b.Cells {
		resp.Cells[i] = make([]cellResponse, len(row))
		for j, cell := range row {
			resp.Cells[i][j] = cellResponse{
				Answered:    cell.Answered,
				PlayerAlias: cell.PlayerAlias,
				TeamName:    cell.TeamName,
				AvatarURL:   cell.AvatarURL,
			}
		}
	}

	return resp
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
