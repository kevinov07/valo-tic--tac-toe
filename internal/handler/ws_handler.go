package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"valo-tic-tac-toe-backend/internal/service"
)

var allowedOrigins = getAllowedOrigins()

func getAllowedOrigins() []string {
	raw := os.Getenv("CORS_ORIGINS")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		if allowedOrigins == nil {
			return true
		}
		origin := r.Header.Get("Origin")
		for _, o := range allowedOrigins {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
		return false
	},
}

type wsMessage struct {
	Type        string `json:"type"`
	Code        string `json:"code,omitempty"`
	Row         int    `json:"row,omitempty"`
	Col         int    `json:"col,omitempty"`
	PlayerAlias string `json:"player_alias,omitempty"`
}

type wsResponse struct {
	Type        string        `json:"type"`
	Code        string        `json:"code,omitempty"`
	PlayerIndex int           `json:"player_index"`
	YourTurn    *bool         `json:"your_turn,omitempty"`
	Board       any           `json:"board,omitempty"`
	Row         int           `json:"row"`
	Col         int           `json:"col"`
	Correct     *bool         `json:"correct,omitempty"`
	PlayerAlias string        `json:"player_alias,omitempty"`
	TeamName    string        `json:"team_name,omitempty"`
	AvatarURL   string        `json:"avatar_url,omitempty"`
	Winner      int           `json:"winner"`
	WinLine     []int         `json:"win_line,omitempty"`
	FromPlayer  int           `json:"from_player,omitempty"`
	Message     string        `json:"message,omitempty"`
}

type playerConn struct {
	conn       *websocket.Conn
	playerIdx  int
	mu         sync.Mutex
}

type WSHandler struct {
	roomManager *service.RoomManager
	rooms       map[string][2]*playerConn
	mu          sync.RWMutex
}

func NewWSHandler(roomManager *service.RoomManager) *WSHandler {
	return &WSHandler{
		roomManager: roomManager,
		rooms:       make(map[string][2]*playerConn),
	}
}

func (h *WSHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	log.Printf("[ws] cliente conectado desde %s", r.RemoteAddr)

	var currentRoom string
	var currentPlayerIdx int
	defer func() {
		log.Printf("[ws] cliente desconectado %s (room=%s, player=%d)", r.RemoteAddr, currentRoom, currentPlayerIdx)
		conn.Close()
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if currentRoom != "" {
				log.Printf("[ws] cliente %s abandonó room %s (player %d)", r.RemoteAddr, currentRoom, currentPlayerIdx)
				h.leaveRoom(currentRoom, currentPlayerIdx)
			}
			break
		}

		var msg wsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[ws] mensaje inválido de %s: %s", r.RemoteAddr, string(raw))
			h.send(conn, wsResponse{Type: "error", Message: "mensaje inválido"})
			continue
		}

		log.Printf("[ws] mensaje de %s: type=%s code=%s row=%d col=%d alias=%s", r.RemoteAddr, msg.Type, msg.Code, msg.Row, msg.Col, msg.PlayerAlias)

		switch msg.Type {
		case "create_room":
			room, err := h.roomManager.CreateRoom()
			if err != nil {
				log.Printf("[ws] error create_room: %v", err)
				h.send(conn, wsResponse{Type: "error", Message: err.Error()})
				continue
			}
			currentRoom = room.Code
			currentPlayerIdx = 0
			h.registerConn(room.Code, 0, conn)
			log.Printf("[ws] sala %s creada por %s", room.Code, r.RemoteAddr)
			notYourTurn := false
			h.send(conn, wsResponse{
				Type:        "room_created",
				Code:        room.Code,
				PlayerIndex: 0,
				YourTurn:    &notYourTurn,
			})

		case "join_room":
			room, idx, err := h.roomManager.JoinRoom(msg.Code)
			if err != nil {
				log.Printf("[ws] error join_room code=%s: %v", msg.Code, err)
				h.send(conn, wsResponse{Type: "error", Message: err.Error()})
				continue
			}
			currentRoom = room.Code
			currentPlayerIdx = idx
			h.registerConn(room.Code, idx, conn)
			log.Printf("[ws] jugador %d se unió a sala %s desde %s", idx, room.Code, r.RemoteAddr)

			boardResp := boardToResponse(room.Board)
			yourTurn := idx == room.Turn

			h.send(conn, wsResponse{
				Type:        "room_joined",
				Code:        room.Code,
				PlayerIndex: idx,
				Board:       boardResp,
				YourTurn:    &yourTurn,
			})
			log.Printf("[ws] enviado room_joined a jugador %d en sala %s (yourTurn=%v)", idx, room.Code, yourTurn)

			if other := h.getConn(room.Code, 0); other != nil {
				otherTurn := 0 == room.Turn
				h.send(other.conn, wsResponse{
					Type:     "opponent_joined",
					Board:    boardResp,
					YourTurn: &otherTurn,
				})
				log.Printf("[ws] enviado opponent_joined a jugador 0 en sala %s (yourTurn=%v)", room.Code, otherTurn)
			} else {
				log.Printf("[ws] ADVERTENCIA: no se encontró conexión del jugador 0 en sala %s", room.Code)
			}

		case "guess":
			if currentRoom == "" {
				h.send(conn, wsResponse{Type: "error", Message: "no estás en una sala"})
				continue
			}
			room := h.roomManager.GetRoom(currentRoom)
			if room == nil {
				h.send(conn, wsResponse{Type: "error", Message: "sala no encontrada"})
				continue
			}

			correct, player, gameOver, winner, winLine, err := h.roomManager.HandleGuess(room, currentPlayerIdx, msg.Row, msg.Col, msg.PlayerAlias)
			if err != nil {
				log.Printf("[ws] error guess en sala %s (player %d): %v", currentRoom, currentPlayerIdx, err)
				h.send(conn, wsResponse{Type: "error", Message: err.Error()})
				continue
			}
			log.Printf("[ws] guess en sala %s: player=%d correct=%v gameOver=%v winner=%d", currentRoom, currentPlayerIdx, correct, gameOver, winner)

			teamName := ""
			avatarURL := ""
			if player != nil {
				teamName = player.CurrentTeamName
				avatarURL = player.AvatarURL
			}

			correctPtr := correct
			baseResp := wsResponse{
				Type:        "guess_result",
				Row:         msg.Row,
				Col:         msg.Col,
				Correct:     &correctPtr,
				PlayerAlias: msg.PlayerAlias,
				TeamName:    teamName,
				AvatarURL:   avatarURL,
				PlayerIndex: currentPlayerIdx,
			}

			if gameOver {
				baseResp.Winner = winner
				baseResp.WinLine = winLine
				baseResp.Type = "game_over"
			}

			h.mu.RLock()
			roomConns, ok := h.rooms[currentRoom]
			h.mu.RUnlock()
			if ok {
				for idx, pc := range roomConns {
					if pc == nil {
						log.Printf("[ws] sala %s: jugador %d no tiene conexión, saltando", currentRoom, idx)
						continue
					}
					if !gameOver {
						yourTurn := idx == room.Turn
						baseResp.YourTurn = &yourTurn
					}
					log.Printf("[ws] enviando %s a jugador %d en sala %s (correct=%v player_alias=%s yourTurn=%v)", baseResp.Type, idx, currentRoom, baseResp.Correct, baseResp.PlayerAlias, baseResp.YourTurn)
					h.send(pc.conn, baseResp)
				}
			} else {
				log.Printf("[ws] sala %s no encontrada en handler al enviar guess_result", currentRoom)
			}

		case "play_again_request":
			log.Printf("[ws] play_again_request en sala %s por jugador %d", currentRoom, currentPlayerIdx)
			if other := h.getOtherConn(currentRoom, currentPlayerIdx); other != nil {
				h.send(other.conn, wsResponse{
					Type:       "play_again_requested",
					FromPlayer: currentPlayerIdx,
				})
			}

		case "request_reset":
			log.Printf("[ws] request_reset en sala %s por jugador %d", currentRoom, currentPlayerIdx)
			if other := h.getOtherConn(currentRoom, currentPlayerIdx); other != nil {
				h.send(other.conn, wsResponse{
					Type:       "reset_requested",
					FromPlayer: currentPlayerIdx,
				})
			}

		case "accept_play_again", "accept_reset":
			room := h.roomManager.GetRoom(currentRoom)
			if room == nil {
				h.send(conn, wsResponse{Type: "error", Message: "sala no encontrada"})
				continue
			}
			if err := h.roomManager.ResetRoom(room); err != nil {
				h.send(conn, wsResponse{Type: "error", Message: err.Error()})
				continue
			}
			log.Printf("[ws] %s en sala %s por jugador %d", msg.Type, currentRoom, currentPlayerIdx)
			h.broadcastGameRestarted(room, currentRoom)

		case "decline_play_again":
			if other := h.getOtherConn(currentRoom, currentPlayerIdx); other != nil {
				h.send(other.conn, wsResponse{
					Type:    "play_again_declined",
					Message: "El oponente rechazó jugar de nuevo",
				})
			}

		case "decline_reset":
			if other := h.getOtherConn(currentRoom, currentPlayerIdx); other != nil {
				h.send(other.conn, wsResponse{
					Type:    "reset_declined",
					Message: "El oponente rechazó reiniciar la partida",
				})
			}

		case "leave_room":
			if currentRoom != "" {
				h.leaveRoom(currentRoom, currentPlayerIdx)
				currentRoom = ""
			}
		}
	}
}

func (h *WSHandler) registerConn(code string, idx int, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[code]
	room[idx] = &playerConn{conn: conn, playerIdx: idx}
	h.rooms[code] = room
}

func (h *WSHandler) getConn(code string, idx int) *playerConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room, ok := h.rooms[code]
	if !ok {
		return nil
	}
	return room[idx]
}

func (h *WSHandler) broadcastRoom(code string, resp wsResponse) {
	h.mu.RLock()
	room, ok := h.rooms[code]
	h.mu.RUnlock()
	if !ok {
		return
	}
	for _, pc := range room {
		if pc != nil {
			h.send(pc.conn, resp)
		}
	}
}

func (h *WSHandler) leaveRoom(code string, playerIdx int) {
	h.roomManager.RemovePlayer(code, playerIdx)

	h.mu.Lock()
	room, ok := h.rooms[code]
	if ok {
		if other := room[1-playerIdx]; other != nil {
			h.send(other.conn, wsResponse{Type: "opponent_left"})
		}
		delete(h.rooms, code)
	}
	h.mu.Unlock()
}

func (h *WSHandler) cleanupRoom(code string) {
	h.mu.Lock()
	delete(h.rooms, code)
	h.mu.Unlock()
}

func (h *WSHandler) getOtherConn(code string, playerIdx int) *playerConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room, ok := h.rooms[code]
	if !ok {
		return nil
	}
	return room[1-playerIdx]
}

func (h *WSHandler) broadcastGameRestarted(room *service.Room, code string) {
	boardResp := boardToResponse(room.Board)
	h.mu.RLock()
	roomConns, ok := h.rooms[code]
	h.mu.RUnlock()
	if !ok {
		return
	}
	for idx, pc := range roomConns {
		if pc == nil {
			continue
		}
		yourTurn := idx == room.Turn
		h.send(pc.conn, wsResponse{
			Type:        "game_restarted",
			Board:       boardResp,
			PlayerIndex: idx,
			YourTurn:    &yourTurn,
		})
	}
}

func (h *WSHandler) send(conn *websocket.Conn, resp wsResponse) {
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(resp); err != nil {
		log.Printf("ws write error: %v", err)
	}
}

func boardToResponse(b *service.Board) boardResponse {
	rows := make([]categoryResponse, 3)
	cols := make([]categoryResponse, 3)
	cells := make([][]cellResponse, 3)

	for i := 0; i < 3; i++ {
		rows[i] = categoryResponse{Kind: b.Rows[i].Kind, Value: b.Rows[i].Value, Label: b.Rows[i].Label}
		cols[i] = categoryResponse{Kind: b.Cols[i].Kind, Value: b.Cols[i].Value, Label: b.Cols[i].Label}
		cells[i] = make([]cellResponse, 3)
		for j := 0; j < 3; j++ {
			cell := b.Cells[i][j]
			cells[i][j] = cellResponse{
				Answered:       cell.Answered,
				PlayerAlias:    cell.PlayerAlias,
				TeamName:       cell.TeamName,
				AvatarURL:      cell.AvatarURL,
				LastGuessAlias: cell.LastGuessAlias,
				LastGuessWrong: cell.LastGuessWrong,
				OwnerPlayer:    cell.OwnerPlayer,
			}
		}
	}

	return boardResponse{
		ID:    b.ID,
		Rows:  rows,
		Cols:  cols,
		Cells: cells,
	}
}
