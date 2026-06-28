package model

// PlayerRole refleja el ENUM player_role de schema.sql, incluyendo "Flex"
// (ver decisión en el chat: Flex es distinto de "sin rol", representa un
// jugador cuyo uso de agentes no está dominado por un solo rol).
type PlayerRole string

const (
	RoleDuelist    PlayerRole = "Duelist"
	RoleController PlayerRole = "Controller"
	RoleInitiator  PlayerRole = "Initiator"
	RoleSentinel   PlayerRole = "Sentinel"
	RoleFlex       PlayerRole = "Flex"
)

// Player es la forma "plana" de un jugador para el motor de juego: solo
// los campos que realmente se usan para evaluar categorías (no incluye
// titles, que son N:1 y se consultan aparte cuando la categoría las
// necesita; ver CategoryKind más abajo).
type Player struct {
	ID                 int64
	Alias              string
	RealName           string
	CountryCode        string
	CurrentTeamID      int64
	CurrentTeamName    string
	Role               *PlayerRole // puntero porque puede ser NULL en la BD
	SignatureAgentName string
	IsCaptain          bool
	AvatarURL          string
	CurrentTeamLogo    string   // URL del logo del equipo actual
	PastTeamNames      []string // equipos anteriores del jugador
	Titles             []string // títulos ganados (raw_label de player_titles)
}

// Team es la forma mínima de equipo que el motor de juego necesita.
type Team struct {
	ID   int64
	Name string
	Tag  string
	Logo string
}

// CategoryKind identifica qué tipo de atributo evalúa una categoría.
// Esto determina CÓMO se evalúa "¿este jugador cumple esta categoría?"
// y de dónde sale la lista de jugadores candidatos.
type CategoryKind string

const (
	KindCurrentTeam CategoryKind = "current_team"
	KindPastTeam    CategoryKind = "past_team"
	KindCountry     CategoryKind = "country"
	KindRole        CategoryKind = "role"
	KindIsCaptain   CategoryKind = "is_captain"
	KindTeammate    CategoryKind = "teammate"
	KindAgent       CategoryKind = "agent"
	// Title queda fuera del MVP por ahora (ver discusión: títulos son
	// buena categoría pero requieren una decisión de UX adicional sobre
	// cómo se presenta "ganó X" como etiqueta corta de columna/fila;
	// se deja el Kind definido para no romper el modelo cuando se agregue)
	KindTitle CategoryKind = "title"
)

// Category representa una fila o columna del tablero: un tipo de atributo
// (Kind) más el valor específico (Value) que el jugador debe cumplir.
// Ej: Kind=KindCountry, Value="ma" (Marruecos)
//
//	Kind=KindCurrentTeam, Value="Sentinels"
//	Kind=KindRole, Value="Controller"
type Category struct {
	Kind  CategoryKind
	Value string
	// Label es el texto legible para mostrar en el frontend, ej:
	// "Jugó para Sentinels", "Es de Marruecos", "Juega Controller".
	// Se separa de Value porque Value debe ser estable para matchear
	// contra la BD, mientras Label puede cambiar de redacción libremente.
	Label string
	// ImageUrl es una URL opcional a una imagen representativa (bandera,
	// logo de equipo, etc.) para mostrar en la celda de categoría.
	ImageUrl string
}
