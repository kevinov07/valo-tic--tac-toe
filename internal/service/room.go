package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"time"

	"valo-tic-tac-toe-backend/internal/model"
)

type RoomStatus string

const (
	RoomWaiting  RoomStatus = "waiting"
	RoomPlaying  RoomStatus = "playing"
	RoomFinished RoomStatus = "finished"
)

type PlayerSlot struct {
	Connected bool
	Alias     string
}

type RoomSettings struct {
	StealEnabled bool
}

type Room struct {
	Code      string
	Players   [2]*PlayerSlot
	Board     *Board
	Turn      int
	Status    RoomStatus
	Settings  RoomSettings
	CreatedAt time.Time
}

func DefaultRoomSettings() RoomSettings {
	return RoomSettings{StealEnabled: true}
}

type RoomManager struct {
	mu     sync.RWMutex
	rooms  map[string]*Room
	engine *GameEngine
}

func NewRoomManager(engine *GameEngine) *RoomManager {
	return &RoomManager{
		rooms:  make(map[string]*Room),
		engine: engine,
	}
}

func (rm *RoomManager) CreateRoom() (*Room, error) {
	code, err := generateRoomCode()
	if err != nil {
		return nil, err
	}

	room := &Room{
		Code: code,
		Players: [2]*PlayerSlot{
			{Connected: true},
			{},
		},
		Status:    RoomWaiting,
		Settings:  DefaultRoomSettings(),
		CreatedAt: time.Now(),
	}

	rm.mu.Lock()
	rm.rooms[code] = room
	rm.mu.Unlock()

	return room, nil
}

func (rm *RoomManager) JoinRoom(code string) (*Room, int, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, ok := rm.rooms[code]
	if !ok {
		return nil, -1, errors.New("sala no encontrada")
	}
	if room.Status != RoomWaiting {
		return nil, -1, errors.New("la sala ya está en juego o terminó")
	}
	if room.Players[1].Connected {
		return nil, -1, errors.New("la sala está llena")
	}

	room.Players[1].Connected = true
	room.Status = RoomPlaying

	board, err := rm.engine.GenerateBoard(code)
	if err != nil {
		room.Status = RoomWaiting
		room.Players[1].Connected = false
		return nil, -1, errors.New("error al generar el tablero")
	}
	room.Board = board
	room.Turn = 0

	return room, 1, nil
}

func (rm *RoomManager) GetRoom(code string) *Room {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.rooms[code]
}

func (rm *RoomManager) ResetRoom(room *Room) error {
	board, err := rm.engine.GenerateBoard(room.Code)
	if err != nil {
		return err
	}
	room.Board = board
	room.Turn = 0
	room.Status = RoomPlaying
	return nil
}

func (rm *RoomManager) HandleGuess(room *Room, playerIdx int, row, col int, playerAlias string) (correct bool, matchedPlayer *model.Player, gameOver bool, winner int, winLine []int, err error) {
	if room.Status != RoomPlaying {
		return false, nil, false, -1, nil, errors.New("la partida no está en juego")
	}
	if room.Turn != playerIdx {
		return false, nil, false, -1, nil, errors.New("no es tu turno")
	}

	// VerifyGuess no modifica el estado de la celda ni rechaza celdas ya respondidas,
	// permitiendo el steal: cualquier celda se puede responder en cualquier momento.
	correct, matchedPlayer, err = rm.engine.VerifyGuess(room.Board, row, col, playerAlias)
	if err != nil {
		return false, nil, false, -1, nil, err
	}

	if correct {
		room.Board.Cells[row][col] = CellState{
			Answered:       true,
			PlayerAlias:    matchedPlayer.Alias,
			TeamName:       matchedPlayer.CurrentTeamName,
			AvatarURL:      matchedPlayer.AvatarURL,
			LastGuessAlias: matchedPlayer.Alias,
			LastGuessWrong: false,
			OwnerPlayer:    playerIdx,
		}

		// Win check: el jugador actual necesita 3 en raya de celdas que LE PERTENEZCAN
		owned := [9]bool{}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				owned[i*3+j] = room.Board.Cells[i][j].OwnerPlayer == playerIdx
			}
		}

		for _, combo := range WinningCombinations {
			if owned[combo[0]] && owned[combo[1]] && owned[combo[2]] {
				room.Status = RoomFinished
				return true, matchedPlayer, true, playerIdx, combo[:], nil
			}
		}

		// Draw check: todas las celdas tienen al menos un OwnerPlayer != -1
		allClaimed := true
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				if room.Board.Cells[i][j].OwnerPlayer == -1 {
					allClaimed = false
					break
				}
			}
			if !allClaimed {
				break
			}
		}
		if allClaimed {
			// Si todas las celdas están reclamadas pero nadie tiene 3 en raya, es empate
			room.Status = RoomFinished
			return true, matchedPlayer, true, -1, nil, nil
		}
	} else {
		// Respuesta incorrecta: actualizar display, pero NO cambiar el OwnerPlayer
		cell := room.Board.Cells[row][col]
		cell.LastGuessAlias = playerAlias
		cell.LastGuessWrong = true
		room.Board.Cells[row][col] = cell
	}

	room.Turn = 1 - playerIdx
	return correct, matchedPlayer, false, -1, nil, nil
}

func (rm *RoomManager) RemovePlayer(code string, playerIdx int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, ok := rm.rooms[code]
	if !ok {
		return
	}
	room.Players[playerIdx].Connected = false
	if room.Status == RoomWaiting {
		delete(rm.rooms, code)
		return
	}
	room.Status = RoomFinished
}

var WinningCombinations = [8][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
	{0, 4, 8}, {2, 4, 6},
}

func generateRoomCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 6)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		code[i] = charset[n.Int64()]
	}
	return string(code), nil
}
