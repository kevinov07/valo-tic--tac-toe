package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"valo-tic-tac-toe-backend/internal/model"
)

// vctTeamNames son los equipos VCT International League que usamos como
// filtro para equipos pasados.
var vctTeamNames = []string{
	"100 Thieves",
	"Cloud9",
	"FURIA",
	"G2 Esports",
	"KRÜ Esports",
	"Leviatán",
	"LOUD",
	"MIBR",
	"NRG",
	"Sentinels",
	"BBL Esports",
	"FNATIC",
	"FUT Esports",
	"GIANTX",
	"Karmine Corp",
	"Natus Vincere",
	"Team Heretics",
	"Team Vitality",
	"DetonatioN FocusMe",
	"Gen.G",
	"Global Esports",
	"KIWOOM DRX",
	"Paper Rex",
	"Rex Regum Qeon",
	"T1",
	"Talon Esports",
	"Team Secret",
	"ZETA DIVISION",
	"All Gamers",
	"Bilibili Gaming",
	"Dragon Ranger Gaming",
	"Edward Gaming",
	"FunPlus Phoenix",
	"JDG Esports",
	"LNG Esports",
	"Nova Esports",
	"Titan Esports Club",
	"Trace Esports",
	"TyLoo",
	"Wolves Esports",
}

var vctSet map[string]bool

func vctLookup(name string) bool {
	if vctSet == nil {
		vctSet = make(map[string]bool, len(vctTeamNames))
		for _, n := range vctTeamNames {
			vctSet[strings.ToLower(n)] = true
		}
	}
	return vctSet[strings.ToLower(strings.TrimSpace(name))]
}

// majorTitleFn es una función que determina si un título (raw_label) es
// un torneo internacional/regional de alto nivel.
type majorTitleFn func(string) bool

var majorTitleChecks = []majorTitleFn{
	// Masters internacionales: "Champions Tour X: Masters Madrid (2024)" ✓
	// Regionales: "CIS Stage 1: Masters (2021)" ✗
	func(s string) bool {
		i := strings.Index(s, "Masters")
		if i < 0 {
			return false
		}
		// Si después de "Masters" viene " (" no hay locación → regional
		after := s[i+len("Masters"):]
		return !strings.HasPrefix(after, " (")
	},
	// World championship: "Valorant Champions X" o ": Champions"
	func(s string) bool { return strings.Contains(s, "Valorant Champions") || strings.Contains(s, ": Champions") },
	// LOCK//IN
	func(s string) bool { return strings.Contains(s, "LOCK//IN") || strings.Contains(s, "LOCK IN") },
	// VCT regional: VCT + una de las 4 regiones
	func(s string) bool {
		if !strings.Contains(s, "VCT") {
			return false
		}
		regions := []string{"Americas", "EMEA", "Pacific", "China"}
		for _, r := range regions {
			if strings.Contains(s, r) {
				return true
			}
		}
		return false
	},
}

func isMajorTitle(rawLabel string) bool {
	for _, fn := range majorTitleChecks {
		if fn(rawLabel) {
			return true
		}
	}
	return false
}

// PlayerRepository implementa service.PlayerStore contra Postgres.
type PlayerRepository struct {
	db *sql.DB
}

func NewPlayerRepository(db *sql.DB) *PlayerRepository {
	return &PlayerRepository{db: db}
}

func (r *PlayerRepository) AllPlayers() ([]model.Player, error) {
	const query = `
		SELECT
			p.id,
			p.alias,
			COALESCE(p.real_name, ''),
			COALESCE(p.country_code, ''),
			COALESCE(p.current_team_id, 0),
			COALESCE(t.name, ''),
			p.role::text,
			COALESCE(a.name, ''),
			p.is_captain,
			COALESCE(p.avatar_url, '')
		FROM players p
		LEFT JOIN teams t ON t.id = p.current_team_id
		LEFT JOIN agents a ON a.id = p.signature_agent_id
		WHERE p.is_active = true
		ORDER BY p.alias
	`

	rows, err := r.db.QueryContext(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("consultar jugadores: %w", err)
	}
	defer rows.Close()

	var players []model.Player
	for rows.Next() {
		var p model.Player
		var role sql.NullString

		if err := rows.Scan(
			&p.ID,
			&p.Alias,
			&p.RealName,
			&p.CountryCode,
			&p.CurrentTeamID,
			&p.CurrentTeamName,
			&role,
			&p.SignatureAgentName,
			&p.IsCaptain,
			&p.AvatarURL,
		); err != nil {
			return nil, fmt.Errorf("leer fila de jugador: %w", err)
		}

		if role.Valid {
			r := model.PlayerRole(role.String)
			p.Role = &r
		}

		players = append(players, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar jugadores: %w", err)
	}

	// Cargar equipos pasados para todos los jugadores activos
	if err := r.loadPastTeams(context.Background(), players); err != nil {
		return nil, fmt.Errorf("cargar equipos pasados: %w", err)
	}

	// Cargar títulos para todos los jugadores activos
	if err := r.loadTitles(context.Background(), players); err != nil {
		return nil, fmt.Errorf("cargar títulos: %w", err)
	}

	return players, nil
}

func (r *PlayerRepository) loadPastTeams(ctx context.Context, players []model.Player) error {
	if len(players) == 0 {
		return nil
	}

	const query = `
		SELECT p.id, ppt.raw_team_name
		FROM player_past_teams ppt
		JOIN players p ON p.id = ppt.player_id
		WHERE p.is_active = true
		ORDER BY p.id, ppt.sort_order
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("consultar equipos pasados: %w", err)
	}
	defer rows.Close()

	playerTeams := make(map[int64][]string)
	for rows.Next() {
		var playerID int64
		var teamName string
		if err := rows.Scan(&playerID, &teamName); err != nil {
			return fmt.Errorf("leer fila de equipo pasado: %w", err)
		}
		// Filtrar nombres de equipo sospechosamente largos
		// (la fuente a veces pega fechas al nombre, ej: "Oxygen AcademyJanuary 2022")
		// y solo conservar equipos VCT International League (Tier 1).
		if len(teamName) > 0 && len(teamName) <= 40 && vctLookup(teamName) {
			playerTeams[playerID] = append(playerTeams[playerID], teamName)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterar equipos pasados: %w", err)
	}

	for i := range players {
		if teams, ok := playerTeams[players[i].ID]; ok {
			players[i].PastTeamNames = teams
		} else {
			players[i].PastTeamNames = []string{}
		}
	}

	return nil
}

func (r *PlayerRepository) loadTitles(ctx context.Context, players []model.Player) error {
	if len(players) == 0 {
		return nil
	}

	const query = `
		SELECT pt.player_id, pt.raw_label
		FROM player_titles pt
		JOIN players p ON p.id = pt.player_id
		WHERE p.is_active = true
		ORDER BY pt.player_id, pt.event_year DESC NULLS LAST
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("consultar títulos: %w", err)
	}
	defer rows.Close()

	playerTitles := make(map[int64][]string)
	for rows.Next() {
		var playerID int64
		var rawLabel string
		if err := rows.Scan(&playerID, &rawLabel); err != nil {
			return fmt.Errorf("leer fila de título: %w", err)
		}
		if isMajorTitle(rawLabel) {
			playerTitles[playerID] = append(playerTitles[playerID], rawLabel)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterar títulos: %w", err)
	}

	for i := range players {
		if titles, ok := playerTitles[players[i].ID]; ok {
			players[i].Titles = titles
		} else {
			players[i].Titles = []string{}
		}
	}

	return nil
}

func (r *PlayerRepository) SearchPlayers(query string, limit int) ([]model.Player, error) {
	if limit <= 0 {
		limit = 10
	}

	const sqlQuery = `
		SELECT
			p.id,
			p.alias,
			COALESCE(p.real_name, ''),
			COALESCE(p.country_code, ''),
			COALESCE(p.current_team_id, 0),
			COALESCE(t.name, ''),
			p.role::text,
			COALESCE(a.name, ''),
			p.is_captain,
			COALESCE(p.avatar_url, '')
		FROM players p
		LEFT JOIN teams t ON t.id = p.current_team_id
		LEFT JOIN agents a ON a.id = p.signature_agent_id
		WHERE p.is_active = true
		  AND p.alias ILIKE '%' || $1 || '%'
		ORDER BY
			CASE WHEN p.alias ILIKE $1 || '%' THEN 0 ELSE 1 END,
			p.alias
		LIMIT $2
	`

	rows, err := r.db.QueryContext(context.Background(), sqlQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("buscar jugadores: %w", err)
	}
	defer rows.Close()

	var players []model.Player
	for rows.Next() {
		var p model.Player
		var role sql.NullString

		if err := rows.Scan(
			&p.ID,
			&p.Alias,
			&p.RealName,
			&p.CountryCode,
			&p.CurrentTeamID,
			&p.CurrentTeamName,
			&role,
			&p.SignatureAgentName,
			&p.IsCaptain,
			&p.AvatarURL,
		); err != nil {
			return nil, fmt.Errorf("leer fila de jugador: %w", err)
		}

		if role.Valid {
			r := model.PlayerRole(role.String)
			p.Role = &r
		}

		players = append(players, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterar jugadores: %w", err)
	}

	return players, nil
}
