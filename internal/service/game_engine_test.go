package service

import (
	"math/rand"
	"testing"

	"valo-tic-tac-toe-backend/internal/model"
)

// fakePlayerStore implementa PlayerStore con una lista fija en memoria,
// para poder probar el motor de juego sin tocar Postgres.
type fakePlayerStore struct {
	players []model.Player
}

func (f *fakePlayerStore) AllPlayers() ([]model.Player, error) {
	return f.players, nil
}

func roleOf(r model.PlayerRole) *model.PlayerRole {
	return &r
}

// realDatasetSample reconstruye un subconjunto REAL de tu dataset.json
// (Sentinels, Paper Rex, NRG), incluyendo a johnqt con Role=Controller
// e is_captain=true, y un caso con Role=nil para representar datos
// insuficientes. Incluye equipos pasados para probar categorías past_team y teammate.
func realDatasetSample() []model.Player {
	return []model.Player{
		// Sentinels — johnqt tiene equipos pasados, Reduxx no
		{Alias: "johnqt", CountryCode: "ma", CurrentTeamName: "Sentinels", Role: roleOf(model.RoleController), SignatureAgentName: "harbor", IsCaptain: true, PastTeamNames: []string{"M80", "Ghost Gaming"}},
		{Alias: "Reduxx", CountryCode: "us", CurrentTeamName: "Sentinels", Role: roleOf(model.RoleDuelist), SignatureAgentName: "jett", IsCaptain: false},
		{Alias: "Jerrwin", CountryCode: "in", CurrentTeamName: "Sentinels", Role: roleOf(model.RoleDuelist), SignatureAgentName: "raze", IsCaptain: false},
		{Alias: "cortezia", CountryCode: "br", CurrentTeamName: "Sentinels", Role: roleOf(model.RoleController), SignatureAgentName: "omen", IsCaptain: false},
		{Alias: "JonahP", CountryCode: "ca", CurrentTeamName: "Sentinels", Role: roleOf(model.RoleInitiator), SignatureAgentName: "skye", IsCaptain: false},

		// Paper Rex — Jinggg tiene equipos pasados
		{Alias: "invy", CountryCode: "ph", CurrentTeamName: "Paper Rex", Role: roleOf(model.RoleInitiator), SignatureAgentName: "sova", IsCaptain: false},
		{Alias: "Jinggg", CountryCode: "sg", CurrentTeamName: "Paper Rex", Role: roleOf(model.RoleDuelist), SignatureAgentName: "raze", IsCaptain: false, PastTeamNames: []string{"M80", "Team Flash"}},
		{Alias: "f0rsakeN", CountryCode: "id", CurrentTeamName: "Paper Rex", Role: roleOf(model.RoleDuelist), SignatureAgentName: "jett", IsCaptain: false},
		{Alias: "d4v41", CountryCode: "my", CurrentTeamName: "Paper Rex", Role: roleOf(model.RoleSentinel), SignatureAgentName: "skye", IsCaptain: false},
		{Alias: "something", CountryCode: "ru", CurrentTeamName: "Paper Rex", Role: roleOf(model.RoleDuelist), SignatureAgentName: "jett", IsCaptain: false},

		// NRG
		{Alias: "Ethan", CountryCode: "us", CurrentTeamName: "NRG", Role: roleOf(model.RoleInitiator), SignatureAgentName: "skye", IsCaptain: true},
		{Alias: "keiko", CountryCode: "gb", CurrentTeamName: "NRG", Role: roleOf(model.RoleDuelist), SignatureAgentName: "jett", IsCaptain: false},
		{Alias: "brawk", CountryCode: "us", CurrentTeamName: "NRG", Role: roleOf(model.RoleInitiator), SignatureAgentName: "sova", IsCaptain: false},
		{Alias: "mada", CountryCode: "ca", CurrentTeamName: "NRG", Role: roleOf(model.RoleDuelist), SignatureAgentName: "raze", IsCaptain: false},
		{Alias: "skuba", CountryCode: "us", CurrentTeamName: "NRG", Role: roleOf(model.RoleController), SignatureAgentName: "astra", IsCaptain: false},

		// Caso real de datos insuficientes: Role=nil (distinto de Flex).
		{Alias: "scarce-data-player", CountryCode: "us", CurrentTeamName: "NRG", Role: nil, IsCaptain: false},

		// M80 — equipo pasado compartido por johnqt y Jinggg, para probar teammate
		{Alias: "Zeldris", CountryCode: "us", CurrentTeamName: "M80", Role: roleOf(model.RoleDuelist), SignatureAgentName: "jett", IsCaptain: false},
	}
}

func TestCandidateCategories_DerivesFromRealData(t *testing.T) {
	players := realDatasetSample()
	teammateMap := buildTeammateMap(players)
	cats := candidateCategories(players, teammateMap)

	got := map[model.CategoryKind]int{}
	for _, c := range cats {
		got[c.Kind]++
	}

	// Verificar que cada tipo de categoría tenga al menos la cantidad esperada
	minKinds := map[model.CategoryKind]int{
		model.KindCurrentTeam: 3,  // Sentinels, Paper Rex, NRG (M80 es extra)
		model.KindPastTeam:    3,  // M80, Ghost Gaming, Team Flash
		model.KindCountry:     11, // ma, us, in, br, ca, ph, sg, id, my, ru, gb
		model.KindRole:        4,  // Controller, Duelist, Initiator, Sentinel
		model.KindIsCaptain:   1,
		model.KindTeammate:    1,  // al menos un jugador con compañeros
	}

	for kind, min := range minKinds {
		if got[kind] < min {
			t.Errorf("kind=%s: se esperaban al menos %d categorías, se obtuvieron %d (categorías: %+v)",
				kind, min, got[kind], cats)
		}
	}

	// Verificar que haya una categoría past_team específica
	foundPast := false
	for _, c := range cats {
		if c.Kind == model.KindPastTeam && c.Value == "M80" {
			foundPast = true
			break
		}
	}
	if !foundPast {
		t.Error("debería haber una categoría past_team para M80 (johnqt y Jinggg jugaron allí)")
	}

	// Verificar que exista al menos una categoría teammate
	foundTeammate := false
	for _, c := range cats {
		if c.Kind == model.KindTeammate {
			foundTeammate = true
			break
		}
	}
	if !foundTeammate {
		t.Error("debería haber al menos una categoría teammate")
	}
}

func TestPlayerMatchesCategory_RoleNilNeverMatches(t *testing.T) {
	players := realDatasetSample()
	teammateMap := buildTeammateMap(players)
	var scarcePlayer model.Player
	for _, p := range players {
		if p.Alias == "scarce-data-player" {
			scarcePlayer = p
		}
	}

	roleCat := model.Category{Kind: model.KindRole, Value: "Controller"}
	if playerMatchesCategory(scarcePlayer, roleCat, teammateMap) {
		t.Error("un jugador con Role=nil nunca debería matchear ninguna categoría de rol")
	}
}

func TestCellHasValidAnswer_KnownIntersection(t *testing.T) {
	players := realDatasetSample()
	teammateMap := buildTeammateMap(players)

	row := model.Category{Kind: model.KindCurrentTeam, Value: "Sentinels"}
	col := model.Category{Kind: model.KindRole, Value: "Controller"}
	if !cellHasValidAnswer(players, row, col, teammateMap) {
		t.Error("Sentinels x Controller debería tener a johnqt como respuesta válida")
	}

	row2 := model.Category{Kind: model.KindCurrentTeam, Value: "Paper Rex"}
	col2 := model.Category{Kind: model.KindRole, Value: "Controller"}
	if cellHasValidAnswer(players, row2, col2, teammateMap) {
		t.Error("Paper Rex x Controller no debería tener respuesta en esta muestra")
	}
}

func TestGenerateBoard_AllCellsHaveValidAnswer(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	engine := NewGameEngine(store, rand.New(rand.NewSource(42)))

	board, err := engine.GenerateBoard("test-game-1")
	if err != nil {
		t.Fatalf("no se esperaba error generando el tablero: %v", err)
	}

	players := realDatasetSample()
	teammateMap := buildTeammateMap(players)
	for _, row := range board.Rows {
		for _, col := range board.Cols {
			if !cellHasValidAnswer(players, row, col, teammateMap) {
				t.Errorf("celda inválida generada: fila=%+v col=%+v", row, col)
			}
		}
	}
}

func TestGenerateBoard_NoDuplicateCategoriesWithinRowsOrCols(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	engine := NewGameEngine(store, rand.New(rand.NewSource(7)))

	board, err := engine.GenerateBoard("test-game-2")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range board.Rows {
		key := string(c.Kind) + ":" + c.Value
		if seen[key] {
			t.Errorf("categoría de fila duplicada: %s", key)
		}
		seen[key] = true
	}

	seenCols := map[string]bool{}
	for _, c := range board.Cols {
		key := string(c.Kind) + ":" + c.Value
		if seenCols[key] {
			t.Errorf("categoría de columna duplicada: %s", key)
		}
		seenCols[key] = true
	}
}

func TestGenerateBoard_FailsGracefullyWithTooFewPlayers(t *testing.T) {
	store := &fakePlayerStore{players: []model.Player{
		{Alias: "solo", CountryCode: "us", CurrentTeamName: "TeamX", Role: roleOf(model.RoleDuelist)},
	}}
	engine := NewGameEngine(store, rand.New(rand.NewSource(1)))

	_, err := engine.GenerateBoard("test-game-3")
	if err != ErrCannotGenerateBoard {
		t.Errorf("se esperaba ErrCannotGenerateBoard, se obtuvo: %v", err)
	}
}

func TestCheckGuess_CorrectAnswer(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	engine := NewGameEngine(store, rand.New(rand.NewSource(1)))

	board := &Board{
		Rows: [3]model.Category{
			{Kind: model.KindCurrentTeam, Value: "Sentinels"},
			{Kind: model.KindCurrentTeam, Value: "Paper Rex"},
			{Kind: model.KindCurrentTeam, Value: "NRG"},
		},
		Cols: [3]model.Category{
			{Kind: model.KindRole, Value: "Controller"},
			{Kind: model.KindRole, Value: "Duelist"},
			{Kind: model.KindRole, Value: "Initiator"},
		},
	}

	correct, player, err := engine.CheckGuess(board, 0, 0, "johnqt")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if !correct {
		t.Fatal("johnqt debería ser una respuesta correcta para Sentinels x Controller")
	}
	if player == nil || player.Alias != "johnqt" {
		t.Errorf("se esperaba el jugador johnqt en la respuesta, se obtuvo: %+v", player)
	}
	if !board.Cells[0][0].Answered {
		t.Error("la celda debería quedar marcada como respondida tras un acierto")
	}
}

func TestCheckGuess_IncorrectAnswer_DoesNotMarkCell(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	engine := NewGameEngine(store, rand.New(rand.NewSource(1)))

	board := &Board{
		Rows: [3]model.Category{{Kind: model.KindCurrentTeam, Value: "Sentinels"}},
		Cols: [3]model.Category{{Kind: model.KindRole, Value: "Controller"}},
	}

	correct, player, err := engine.CheckGuess(board, 0, 0, "Jinggg")
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if correct {
		t.Error("Jinggg no debería ser válido para Sentinels x Controller")
	}
	if player != nil {
		t.Error("no debería devolverse un jugador cuando la respuesta es incorrecta")
	}
	if board.Cells[0][0].Answered {
		t.Error("una respuesta incorrecta NO debe marcar la celda como respondida")
	}
}

func TestCheckGuess_AlreadyAnsweredCellReturnsError(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	engine := NewGameEngine(store, rand.New(rand.NewSource(1)))

	board := &Board{
		Rows: [3]model.Category{{Kind: model.KindCurrentTeam, Value: "Sentinels"}},
		Cols: [3]model.Category{{Kind: model.KindRole, Value: "Controller"}},
	}
	board.Cells[0][0] = CellState{Answered: true, PlayerAlias: "johnqt"}

	_, _, err := engine.CheckGuess(board, 0, 0, "johnqt")
	if err != ErrCellAlreadyAnswered {
		t.Errorf("se esperaba ErrCellAlreadyAnswered, se obtuvo: %v", err)
	}
}

func TestCheckGuess_InvalidCellIndexReturnsError(t *testing.T) {
	store := &fakePlayerStore{players: realDatasetSample()}
	engine := NewGameEngine(store, rand.New(rand.NewSource(1)))
	board := &Board{}

	_, _, err := engine.CheckGuess(board, 5, 0, "johnqt")
	if err != ErrInvalidCell {
		t.Errorf("se esperaba ErrInvalidCell, se obtuvo: %v", err)
	}
}
