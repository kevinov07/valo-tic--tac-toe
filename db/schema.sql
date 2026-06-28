-- ============================================================
-- Valo Tic Tac Toe — esquema de base de datos (PostgreSQL)
-- ============================================================
-- Diseño pensado para la consulta central del juego:
--   "dada categoría de FILA y categoría de COLUMNA, ¿qué jugadores
--    cumplen AMBAS?" (intersección, tipo JOIN)
--
-- Decisión de diseño: híbrido entre columnas fijas y tablas relacionadas.
--   - Categorías de cardinalidad 1 por jugador (país, rol, equipo actual,
--     agente insignia) -> columnas/FK directas en `players`. Son las más
--     consultadas y se benefician de índices simples.
--   - Categorías de cardinalidad N por jugador (equipos pasados, títulos)
--     -> tablas relacionadas, porque son listas de tamaño variable.
--   - is_captain/IGL -> columna booleana simple en players, ya que es
--     una propiedad binaria del jugador (ver discusión en el chat sobre
--     por qué sí es confiable a diferencia del rol crudo de la fuente).
--
-- Esto evita un modelo 100% "EAV" (entity-attribute-value) genérico, que
-- sería más flexible para agregar categorías nuevas sin migraciones, pero
-- a costa de queries más lentas y difíciles de razonar/depurar. Para el
-- volumen de este proyecto (decenas/cientos de jugadores), el híbrido es
-- la mejor relación simplicidad/rendimiento/flexibilidad.
-- ============================================================


-- ----------------------------------------------------------------
-- Extensión para búsqueda de texto sin distinguir tildes/mayúsculas
-- (útil para el autocompletado de nombres de jugador en el front).
-- ----------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE EXTENSION IF NOT EXISTS pg_trgm;  -- búsqueda por similitud/parcial eficiente


-- ----------------------------------------------------------------
-- ENUM de rol de juego. Se usa ENUM (no tabla aparte) porque el set
-- de roles de Valorant es estable y pequeño, y un ENUM da validación
-- a nivel de base de datos sin un JOIN extra.
--
-- 'Flex' se agrega como 5to valor (no es un rol oficial del juego,
-- es una etiqueta nuestra) para jugadores donde NINGÚN rol individual
-- domina su uso de agentes (ver agent_roles.py: cuando el rol con más
-- % de uso no supera el umbral de "dominancia", se asigna Flex en
-- lugar de forzar el rol con más %, que sería engañoso con poca
-- confianza). Esto es DISTINTO de role = NULL:
--   - NULL    = no hay datos suficientes para evaluar (pocas rondas)
--   - 'Flex'  = SÍ hay datos suficientes, y la conclusión real es que
--               el jugador reparte su juego entre 2+ roles
-- Mezclar estos dos casos en NULL perdería la categoría "Flex" como
-- categoría jugable en el tablero (ver discusión en el chat).
--
-- Si Riot agrega un 5to rol oficial algún día, se migra con ALTER TYPE.
-- ----------------------------------------------------------------
DO $$ BEGIN
    CREATE TYPE player_role AS ENUM ('Duelist', 'Controller', 'Initiator', 'Sentinel', 'Flex');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;


-- ----------------------------------------------------------------
-- TEAMS
-- ----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS teams (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,       -- "Sentinels", "Paper Rex"
    tag             TEXT,                        -- "SEN", "PRX" (puede venir vacío en la fuente)
    logo            TEXT,                        -- URL del logo del equipo
    country         TEXT,                        -- "United States", "Europe" (la fuente mezcla país/región aquí)
    vlr_rank        TEXT,                         -- ranking de vlr.gg al momento del seed; informativo, no se usa para queries de juego
    source_team_id  TEXT,                         -- id del equipo en vlrggapi, para poder re-sincronizar después
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_teams_name_trgm ON teams USING gin (name gin_trgm_ops);


-- ----------------------------------------------------------------
-- AGENTS
-- Tabla pequeña y casi estática (28-30 filas). Se usa principalmente
-- para resolver el rol de un agente y, en el futuro, para categorías
-- de tipo "agente" en el tablero si se reactiva esa idea (ver chat:
-- por ahora el MVP solo usa jugadores como respuesta, pero el agente
-- insignia de cada jugador sí se modela como categoría).
-- ----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS agents (
    id      SERIAL PRIMARY KEY,
    name    TEXT NOT NULL UNIQUE,     -- "harbor", "omen" (en minúsculas, como llega de la fuente)
    role    player_role NOT NULL
);


-- ----------------------------------------------------------------
-- PLAYERS
-- ----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS players (
    id                   SERIAL PRIMARY KEY,
    alias                TEXT NOT NULL,             -- "johnqt", "TenZ" — nombre de juego, lo que se usa para responder
    real_name            TEXT,
    country_code         TEXT,                       -- "ma", "us" (código de 2 letras tal cual llega de la fuente)
    current_team_id      INTEGER REFERENCES teams(id) ON DELETE SET NULL,
    role                 player_role,                 -- derivado de agent_stats (ver agent_roles.py); NULL si no hay suficiente confianza
    role_confidence_pct  NUMERIC(5,2),                -- guardado para transparencia/depuración, no para queries de juego
    signature_agent_id   INTEGER REFERENCES agents(id) ON DELETE SET NULL,
    is_captain           BOOLEAN NOT NULL DEFAULT false,  -- usado como proxy de IGL (ver decisión en el chat)
    avatar_url           TEXT,  
    source_player_id     TEXT,                         -- id del jugador en vlrggapi, para re-sincronizar
    is_active            BOOLEAN NOT NULL DEFAULT true,   -- por si luego quieres excluir retirados sin borrar el registro
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Un alias puede repetirse entre distintas fuentes/épocas en teoría,
    -- pero junto con su equipo actual y source_player_id debería ser
    -- prácticamente único. No se fuerza UNIQUE solo en alias por si hay
    -- dos jugadores históricos con el mismo alias (pasa en escenas regionales).
    CONSTRAINT uq_players_source UNIQUE (source_player_id)
);

CREATE INDEX IF NOT EXISTS idx_players_alias_trgm ON players USING gin (alias gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_players_country ON players (country_code);
CREATE INDEX IF NOT EXISTS idx_players_role ON players (role);
CREATE INDEX IF NOT EXISTS idx_players_current_team ON players (current_team_id);

-- Índice compuesto pensado DIRECTAMENTE para la query de intersección
-- más común del juego: "país X + rol Y", "equipo X + país Y", etc.
-- Postgres puede usar índices parciales de esta lista combinada según
-- el patrón de filtro real; se agregan los 3 pares más probables.
CREATE INDEX IF NOT EXISTS idx_players_country_role ON players (country_code, role);
CREATE INDEX IF NOT EXISTS idx_players_team_role ON players (current_team_id, role);
CREATE INDEX IF NOT EXISTS idx_players_team_country ON players (current_team_id, country_code);


-- ----------------------------------------------------------------
-- PAST_TEAMS (cardinalidad N por jugador)
-- ----------------------------------------------------------------
-- NOTA (decisión del usuario, 2026-06): el nombre del equipo pasado
-- llega de la fuente a veces con fechas pegadas sin separador, p.ej.
-- "Oxygen AcademyJanuary 2022 – December 2022". Se guarda CRUDO tal
-- cual en raw_team_name por ahora; team_name_clean queda nullable
-- para cuando se decida limpiar esto (no se hace en este seed inicial).
-- ----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS player_past_teams (
    id              SERIAL PRIMARY KEY,
    player_id       INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    raw_team_name   TEXT NOT NULL,     -- texto crudo de la fuente, sin limpiar
    team_name_clean TEXT,               -- NULL por ahora; reservado para limpieza futura
    team_id         INTEGER REFERENCES teams(id) ON DELETE SET NULL,  -- si se logra resolver a un team existente
    sort_order      INTEGER NOT NULL DEFAULT 0  -- preserva el orden en que vlr.gg los lista (más reciente primero)
);

CREATE INDEX IF NOT EXISTS idx_past_teams_player ON player_past_teams (player_id);
CREATE INDEX IF NOT EXISTS idx_past_teams_raw_name_trgm ON player_past_teams USING gin (raw_team_name gin_trgm_ops);


-- ----------------------------------------------------------------
-- TITLES (cardinalidad N por jugador)
-- ----------------------------------------------------------------
-- Se guarda como texto "Evento (Año)" tal cual se generó en
-- fetch_dataset.py (extract_titles), más el año separado para poder
-- filtrar/ordenar sin parsear el string en cada query.
-- ----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS player_titles (
    id          SERIAL PRIMARY KEY,
    player_id   INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    event_name  TEXT NOT NULL,         -- "Champions Tour 2024: Masters Madrid"
    event_year  INTEGER,                -- 2024 (extraído aparte para poder filtrar/ordenar)
    raw_label   TEXT NOT NULL           -- "Champions Tour 2024: Masters Madrid (2024)", tal cual vino del dataset
);

CREATE INDEX IF NOT EXISTS idx_titles_player ON player_titles (player_id);
CREATE INDEX IF NOT EXISTS idx_titles_event_name_trgm ON player_titles USING gin (event_name gin_trgm_ops);


-- ----------------------------------------------------------------
-- updated_at automático (trigger genérico reutilizable)
-- ----------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_teams_updated_at') THEN
        CREATE TRIGGER trg_teams_updated_at
            BEFORE UPDATE ON teams
            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_players_updated_at') THEN
        CREATE TRIGGER trg_players_updated_at
            BEFORE UPDATE ON players
            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
    END IF;
END $$;


-- ============================================================
-- VISTA DE APOYO: una fila por jugador con todo lo que el
-- generador de tableros necesita para evaluar categorías de
-- cardinalidad 1, sin tener que hacer el JOIN a mano cada vez.
-- (player_past_teams y player_titles quedan fuera por ser N:1;
-- esas se consultan aparte cuando la categoría las requiera.)
-- ============================================================
CREATE OR REPLACE VIEW players_with_team AS
SELECT
    p.id,
    p.alias,
    p.real_name,
    p.country_code,
    p.role,
    p.role_confidence_pct,
    p.is_captain,
    p.is_active,
    p.avatar_url,   
    t.id   AS team_id,
    t.name AS team_name,
    t.tag  AS team_tag,
    a.name AS signature_agent_name
FROM players p
LEFT JOIN teams t  ON t.id = p.current_team_id
LEFT JOIN agents a ON a.id = p.signature_agent_id;
