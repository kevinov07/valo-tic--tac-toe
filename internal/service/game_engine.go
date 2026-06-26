package service

import (
	"errors"
	"math/rand"

	"valo-tic-tac-toe-backend/internal/model"
)

// PlayerStore es la interfaz que el motor de juego necesita del mundo
// exterior (normalmente implementada por internal/repository contra
// Postgres). Se define AQUÍ, en el consumidor, siguiendo el patrón de Go
// de "acepta interfaces, devuelve structs": el motor de juego no sabe ni
// le importa si los jugadores vienen de Postgres, de un mock en tests, o
// de un archivo JSON. Esto es lo que permite probar GenerateBoard y
// CheckGuess con datos falsos en memoria, sin tocar una base de datos real.
type PlayerStore interface {
	// AllPlayers devuelve todos los jugadores activos disponibles para
	// el juego. Para el volumen de este proyecto (decenas/cientos de
	// jugadores) traerlos todos a memoria y filtrar en Go es más simple
	// que construir queries dinámicas por cada combinación de categorías;
	// se puede optimizar después si el dataset crece mucho.
	AllPlayers() ([]model.Player, error)
}

var (
	// ErrCannotGenerateBoard indica que, tras varios intentos, no se logró
	// encontrar una combinación de 6 categorías donde las 9 celdas tengan
	// al menos un jugador válido. Señal de que faltan datos/categorías
	// variadas en la base, no un bug de lógica en sí.
	ErrCannotGenerateBoard = errors.New("no se pudo generar un tablero válido con las categorías disponibles")

	// ErrCellAlreadyAnswered se devuelve si se intenta responder una celda
	// que ya tiene una respuesta correcta registrada en la partida.
	ErrCellAlreadyAnswered = errors.New("esta celda ya fue respondida")

	// ErrInvalidCell se devuelve si row/col están fuera de rango [0,2].
	ErrInvalidCell = errors.New("fila o columna fuera de rango (debe ser 0, 1 o 2)")
)

const boardSize = 3

// Board representa una partida completa: las categorías de fila/columna,
// y el estado de cada una de las 9 celdas. Vive en memoria (ver decisión
// del usuario: sin persistencia en Postgres para el MVP).
type Board struct {
	ID    string
	Rows  [boardSize]model.Category
	Cols  [boardSize]model.Category
	Cells [boardSize][boardSize]CellState
}

// CellState es el estado de una celda individual del tablero.
type CellState struct {
	Answered    bool
	PlayerAlias string // alias con el que se respondió correctamente, si Answered=true
	TeamName    string // dato adicional para mostrar en el frontend al acertar
	AvatarURL   string // URL del avatar del jugador
}

// GameEngine agrupa la lógica de generación y validación de tableros.
// No tiene estado propio más allá de su PlayerStore; las partidas activas
// las gestiona quien lo use (ver internal/handler, que las guarda en un
// map en memoria protegido por mutex).
type GameEngine struct {
	store PlayerStore
	rng   *rand.Rand
}

func NewGameEngine(store PlayerStore, rng *rand.Rand) *GameEngine {
	if rng == nil {
		rng = rand.New(rand.NewSource(rand.Int63()))
	}
	return &GameEngine{store: store, rng: rng}
}

// candidateCategories construye TODAS las categorías candidatas posibles
// a partir de los jugadores reales: una por cada equipo actual distinto,
// una por cada equipo pasado distinto, una por cada país distinto, una por
// cada rol distinto (excluyendo nil), y una por cada jugador que tenga
// compañeros de equipo (teammate).
func candidateCategories(players []model.Player, teammateMap map[string]map[string]bool) []model.Category {
	seenTeams := map[string]bool{}
	seenPastTeams := map[string]bool{}
	seenCountries := map[string]bool{}
	seenRoles := map[model.PlayerRole]bool{}
	seenAgents := map[string]bool{}

	var categories []model.Category

	for _, p := range players {
		if p.CurrentTeamName != "" && !seenTeams[p.CurrentTeamName] {
			seenTeams[p.CurrentTeamName] = true
			categories = append(categories, model.Category{
				Kind:  model.KindCurrentTeam,
				Value: p.CurrentTeamName,
				Label: "Juega en " + p.CurrentTeamName,
			})
		}
		if p.CountryCode != "" && !seenCountries[p.CountryCode] {
			seenCountries[p.CountryCode] = true
			categories = append(categories, model.Category{
				Kind:  model.KindCountry,
				Value: p.CountryCode,
				Label: "País: " + toUpper(p.CountryCode),
			})
		}
		if p.Role != nil && !seenRoles[*p.Role] {
			seenRoles[*p.Role] = true
			categories = append(categories, model.Category{
				Kind:  model.KindRole,
				Value: string(*p.Role),
				Label: "Rol: " + string(*p.Role),
			})
		}
		if p.SignatureAgentName != "" && !seenAgents[p.SignatureAgentName] {
			seenAgents[p.SignatureAgentName] = true
			categories = append(categories, model.Category{
				Kind:  model.KindAgent,
				Value: p.SignatureAgentName,
				Label: p.SignatureAgentName + " (más usado)",
			})
		}
		// Past teams
		for _, pt := range p.PastTeamNames {
			if pt != "" && !seenPastTeams[pt] {
				seenPastTeams[pt] = true
				categories = append(categories, model.Category{
					Kind:  model.KindPastTeam,
					Value: pt,
					Label: "Jugó en " + pt,
				})
			}
		}
	}

	// is_captain es binaria: si hay al menos un capitán entre los
	// jugadores, se agrega como categoría candidata.
	for _, p := range players {
		if p.IsCaptain {
			categories = append(categories, model.Category{
				Kind:  model.KindIsCaptain,
				Value: "true",
				Label: "Es/fue IGL o capitán",
			})
			break
		}
	}

	// Categorías de compañero: una por cada jugador que tenga al menos
	// un compañero de equipo ACTUAL. La etiqueta usa "Juega con"
	// porque la validación solo considera el equipo actual (no pasado).
	seenTeammate := map[string]bool{}
	for _, p := range players {
		if len(teammateMap[p.Alias]) > 0 && !seenTeammate[p.Alias] {
			seenTeammate[p.Alias] = true
			categories = append(categories, model.Category{
				Kind:  model.KindTeammate,
				Value: p.Alias,
				Label: "Juega con " + p.Alias,
			})
		}
	}

	// Categorías de título: una por cada título único que al menos un
	// jugador haya ganado (ej: "Ganó Champions Tour 2024: Champions").
	seenTitles := map[string]bool{}
	for _, p := range players {
		for _, t := range p.Titles {
			if t != "" && !seenTitles[t] {
				seenTitles[t] = true
				categories = append(categories, model.Category{
					Kind:  model.KindTitle,
					Value: t,
					Label: "Ganó " + t,
				})
			}
		}
	}

	return categories
}

func toUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

// buildTeammateMap construye un mapa de alias de jugador -> conjunto de
// aliases de compañeros con los que COMPARTE EQUIPO ACTUALMENTE.
// Solo usa CurrentTeamName para evitar falsos positivos de jugadores
// que pasaron por la misma organización en épocas distintas.
func buildTeammateMap(players []model.Player) map[string]map[string]bool {
	// Equipo actual -> jugadores que juegan en él
	teamPlayers := make(map[string]map[string]bool)
	for _, p := range players {
		if p.CurrentTeamName == "" {
			continue
		}
		if teamPlayers[p.CurrentTeamName] == nil {
			teamPlayers[p.CurrentTeamName] = make(map[string]bool)
		}
		teamPlayers[p.CurrentTeamName][p.Alias] = true
	}

	// Para cada jugador, encontrar sus compañeros actuales
	result := make(map[string]map[string]bool)
	for _, p := range players {
		if p.CurrentTeamName == "" {
			continue
		}
		for teammate := range teamPlayers[p.CurrentTeamName] {
			if teammate != p.Alias {
				if result[p.Alias] == nil {
					result[p.Alias] = make(map[string]bool)
				}
				result[p.Alias][teammate] = true
			}
		}
	}
	return result
}

// playerMatchesCategory evalúa si un jugador cumple una categoría dada.
// Esta es la función que define, en código, qué significa cada Kind.
// Recibe el teammateMap precalculado para evitar reconstruirlo en cada
// llamada para categorías teammate.
func playerMatchesCategory(p model.Player, c model.Category, teammateMap map[string]map[string]bool) bool {
	switch c.Kind {
	case model.KindCurrentTeam:
		return p.CurrentTeamName == c.Value
	case model.KindPastTeam:
		for _, pt := range p.PastTeamNames {
			if pt == c.Value {
				return true
			}
		}
		return false
	case model.KindCountry:
		return p.CountryCode == c.Value
	case model.KindRole:
		return p.Role != nil && string(*p.Role) == c.Value
	case model.KindIsCaptain:
		return p.IsCaptain
	case model.KindTeammate:
		// Verificar si p ha compartido equipo con el jugador c.Value
		if p.Alias == c.Value {
			return false
		}
		return teammateMap[p.Alias][c.Value]
	case model.KindAgent:
		return p.SignatureAgentName == c.Value
	case model.KindTitle:
		for _, t := range p.Titles {
			if t == c.Value {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// cellHasValidAnswer devuelve true si existe al menos un jugador que
// cumpla TANTO la categoría de fila como la de columna.
func cellHasValidAnswer(players []model.Player, row, col model.Category, teammateMap map[string]map[string]bool) bool {
	for _, p := range players {
		if playerMatchesCategory(p, row, teammateMap) && playerMatchesCategory(p, col, teammateMap) {
			return true
		}
	}
	return false
}

// sameUnderlyingAttribute evita combinaciones sin sentido como
// "Rol: Duelist" (fila) x "Rol: Controller" (columna) -- dos categorías
// del MISMO Kind casi nunca tienen jugadores en común (un jugador no
// puede tener dos roles a la vez), y aunque los tuviera, sería una
// categoría poco interesante de jugar. Se evita cruzar categorías del
// mismo Kind entre fila y columna.
func sameKind(a, b model.Category) bool {
	return a.Kind == b.Kind
}

// categoryWeights controla la probabilidad relativa de que cada tipo de
// categoría aparezca en el tablero. Pesos más altos = más probable.
// Usamos un pool con repetición: cada categoría aparece `weight` veces
// en el pool antes de barajar, así las de mayor peso saturan las primeras
// posiciones del orden barajado y el backtracking las elige primero.
var categoryWeights = map[model.CategoryKind]int{
	model.KindCurrentTeam: 3,
	model.KindPastTeam:    2, // la más baja: equipos pasados son difíciles
	model.KindCountry:     5, // alta: mucha intersección con otras categorías
	model.KindRole:        5, // alta: mucha intersección con otras categorías
	model.KindIsCaptain:   2,
	model.KindTeammate:    2,
	model.KindAgent:       4, // alta: muchos agentes, buena intersección
	model.KindTitle:       7,
}

// weightedShuffle baraja categorías usando pesos: cada categoría aparece
// `weight` veces en un pool interno, se baraja, y se deduplica (primera
// ocurrencia gana). El resultado tiende a tener categorías de mayor peso
// más temprano, sesgando el backtracking hacia ellas.
func weightedShuffle(rng *rand.Rand, candidates []model.Category) []model.Category {
	total := 0
	for _, c := range candidates {
		w := categoryWeights[c.Kind]
		if w < 1 {
			w = 1
		}
		total += w
	}
	pool := make([]model.Category, 0, total)
	for _, c := range candidates {
		w := categoryWeights[c.Kind]
		if w < 1 {
			w = 1
		}
		for i := 0; i < w; i++ {
			pool = append(pool, c)
		}
	}
	rng.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})
	seen := make(map[string]bool, len(candidates))
	result := make([]model.Category, 0, len(candidates))
	for _, c := range pool {
		key := string(c.Kind) + "\x00" + c.Value
		if !seen[key] {
			seen[key] = true
			result = append(result, c)
		}
	}
	return result
}

const maxGenerationAttempts = 200

// GenerateBoard intenta construir un tablero de 3x3 donde las 9 celdas
// tienen al menos un jugador válido, eligiendo categorías al azar entre
// las candidatas reales derivadas de los jugadores actuales.
//
// Estrategia: backtracking con shuffle. En cada intento se baraja la
// lista de candidatas y se van agregando una por una a filas o columnas
// (alternando), pero ANTES de aceptar definitivamente las 6 categorías
// se verifica que el tablero parcial siga siendo viable; si una columna
// nueva deja alguna celda sin respuesta posible contra las filas ya
// elegidas, se descarta esa columna y se prueba la siguiente candidata,
// en vez de esperar a tener las 6 categorías completas para descubrir
// que el tablero entero falla (que es lo que hacía una versión anterior,
// y fallaba seguido con pocos jugadores/categorías).
func (e *GameEngine) GenerateBoard(id string) (*Board, error) {
	players, err := e.store.AllPlayers()
	if err != nil {
		return nil, err
	}

	teammateMap := buildTeammateMap(players)
	candidates := candidateCategories(players, teammateMap)
	if len(candidates) < boardSize*2 {
		return nil, ErrCannotGenerateBoard
	}

	for attempt := 0; attempt < maxGenerationAttempts; attempt++ {
		shuffled := weightedShuffle(e.rng, candidates)
		rows, cols, ok := buildValidRowsAndCols(players, shuffled, teammateMap)
		if !ok {
			continue
		}
		// Verificación final de seguridad: buildValidRowsAndCols ya valida
		// incrementalmente, pero esta doble-checada es barata (9 lookups)
		// y deja la invariante "todo tablero devuelto es 100% jugable"
		// blindada ante futuros cambios en la lógica incremental.
		if boardIsFullyValid(players, rows, cols, teammateMap) {
			return &Board{ID: id, Rows: rows, Cols: cols}, nil
		}
	}

	return nil, ErrCannotGenerateBoard
}

// buildValidRowsAndCols intenta construir 3 filas + 3 columnas válidas a
// partir de candidates (ya barajadas) usando backtracking real: si en
// algún punto no hay ninguna candidata que extienda la solución parcial,
// se retrocede y se prueba la siguiente opción en el nivel anterior, en
// vez de abandonar todo el intento.
func buildValidRowsAndCols(players []model.Player, candidates []model.Category, teammateMap map[string]map[string]bool) (rows, cols [boardSize]model.Category, ok bool) {
	var rowPicks, colPicks []model.Category

	if backtrack(players, candidates, &rowPicks, &colPicks, teammateMap) {
		copy(rows[:], rowPicks)
		copy(cols[:], colPicks)
		return rows, cols, true
	}
	return rows, cols, false
}

// backtrack intenta completar rowPicks primero (hasta boardSize), luego
// colPicks, retrocediendo cuando una elección no permite continuar.
// Devuelve true en cuanto encuentra una combinación completa y válida.
func backtrack(players []model.Player, candidates []model.Category, rowPicks, colPicks *[]model.Category, teammateMap map[string]map[string]bool) bool {
	if len(*rowPicks) == boardSize && len(*colPicks) == boardSize {
		return true
	}

	fillingRows := len(*rowPicks) < boardSize

	for _, c := range candidates {
		if fillingRows {
			if containsValue(*rowPicks, c) {
				continue
			}
			*rowPicks = append(*rowPicks, c)
			if backtrack(players, candidates, rowPicks, colPicks, teammateMap) {
				return true
			}
			*rowPicks = (*rowPicks)[:len(*rowPicks)-1] // deshacer y probar siguiente
		} else {
			if containsValue(*colPicks, c) || anySameKind(*rowPicks, c) {
				continue
			}
			if !columnIsValidAgainstRows(players, *rowPicks, c, teammateMap) {
				continue
			}
			*colPicks = append(*colPicks, c)
			if backtrack(players, candidates, rowPicks, colPicks, teammateMap) {
				return true
			}
			*colPicks = (*colPicks)[:len(*colPicks)-1] // deshacer y probar siguiente
		}
	}

	return false
}

func containsValue(list []model.Category, c model.Category) bool {
	for _, existing := range list {
		if existing.Kind == c.Kind && existing.Value == c.Value {
			return true
		}
	}
	return false
}

func anySameKind(list []model.Category, c model.Category) bool {
	for _, existing := range list {
		if sameKind(existing, c) {
			return true
		}
	}
	return false
}

// columnIsValidAgainstRows verifica que una categoría candidata a columna
// tenga al menos un jugador válido contra CADA una de las filas ya elegidas.
func columnIsValidAgainstRows(players []model.Player, rowPicks []model.Category, col model.Category, teammateMap map[string]map[string]bool) bool {
	for _, row := range rowPicks {
		if !cellHasValidAnswer(players, row, col, teammateMap) {
			return false
		}
	}
	return true
}

// boardIsFullyValid verifica que TODAS las 9 celdas tengan al menos un
// jugador válido para la combinación fila x columna correspondiente.
func boardIsFullyValid(players []model.Player, rows, cols [boardSize]model.Category, teammateMap map[string]map[string]bool) bool {
	for _, row := range rows {
		for _, col := range cols {
			if !cellHasValidAnswer(players, row, col, teammateMap) {
				return false
			}
		}
	}
	return true
}

// CheckGuess valida si playerAlias cumple la celda (row, col) del tablero,
// y actualiza el estado de la celda si es correcta. row/col son índices
// 0-2. Devuelve si fue correcto y, si lo fue, los datos del jugador para
// mostrar en el frontend.
func (e *GameEngine) CheckGuess(b *Board, row, col int, playerAlias string) (correct bool, matchedPlayer *model.Player, err error) {
	if row < 0 || row >= boardSize || col < 0 || col >= boardSize {
		return false, nil, ErrInvalidCell
	}
	if b.Cells[row][col].Answered {
		return false, nil, ErrCellAlreadyAnswered
	}

	players, err := e.store.AllPlayers()
	if err != nil {
		return false, nil, err
	}

	teammateMap := buildTeammateMap(players)
	rowCat := b.Rows[row]
	colCat := b.Cols[col]

	for i := range players {
		p := players[i]
		if p.Alias != playerAlias {
			continue
		}
		if playerMatchesCategory(p, rowCat, teammateMap) && playerMatchesCategory(p, colCat, teammateMap) {
			b.Cells[row][col] = CellState{
				Answered:    true,
				PlayerAlias: p.Alias,
				TeamName:    p.CurrentTeamName,
				AvatarURL:   p.AvatarURL,
			}
			return true, &p, nil
		}
		// El alias existe pero no cumple ambas categorías: respuesta
		// incorrecta, no es un error del sistema.
		return false, nil, nil
	}

	// Alias no encontrado entre los jugadores conocidos.
	return false, nil, nil
}
