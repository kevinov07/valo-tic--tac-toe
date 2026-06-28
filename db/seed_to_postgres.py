"""
seed_to_postgres.py
--------------------
Carga el dataset.json (generado por data-fetch/fetch_dataset.py) a una base
de datos PostgreSQL que ya tiene aplicado db/schema.sql.

USO:
    1. Asegúrate de que la base de datos existe y tiene el esquema aplicado:
         psql -d valo_tic_tac_toe -f schema.sql
    2. Configura la conexión por variable de entorno (ver abajo) o edita
       DEFAULT_DSN.
    3. Copia/coloca dataset.json en esta misma carpeta (o usa --dataset para
       indicar otra ruta).
    4. python seed_to_postgres.py

Es IDEMPOTENTE: se puede correr varias veces sin duplicar datos. Usa
UPSERT (INSERT ... ON CONFLICT) basado en:
    - teams.name              (UNIQUE)
    - agents.name              (UNIQUE)
    - players.source_player_id (UNIQUE, ver schema.sql)
Para player_past_teams y player_titles (tablas N:1 sin una clave natural
única evidente), se borran y re-insertan por jugador en cada corrida, para
evitar ir acumulando duplicados si el dataset cambia entre corridas.
"""

import json
import os
import sys
import argparse

import psycopg2
import psycopg2.extras

from dotenv import load_dotenv
load_dotenv()

DEFAULT_DSN = os.environ.get(
    "DATABASE_URL",
    "postgresql://postgres:postgres@localhost:5432/valo_tic_tac_toe",
)


def load_dataset(path: str) -> list:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def upsert_team(cur, team: dict) -> int:
    """Inserta o actualiza un equipo por nombre (UNIQUE). Devuelve su id."""
    cur.execute(
        """
        INSERT INTO teams (name, tag, logo, country, vlr_rank, source_team_id)
        VALUES (%(name)s, %(tag)s, %(logo)s, %(country)s, %(vlr_rank)s, %(source_team_id)s)
        ON CONFLICT (name) DO UPDATE SET
            tag = EXCLUDED.tag,
            logo = EXCLUDED.logo,
            country = EXCLUDED.country,
            vlr_rank = EXCLUDED.vlr_rank,
            source_team_id = EXCLUDED.source_team_id
        RETURNING id
        """,
        {
            "name": team["name"],
            "tag": team.get("tag") or None,
            "logo": team.get("logo_url") or None,
            "country": team.get("country") or None,
            "vlr_rank": team.get("vlr_rank") or None,
            "source_team_id": team.get("source_team_id") or None,
        },
    )
    return cur.fetchone()[0]


def get_or_create_agent_id(cur, agent_name: str, role: str = None) -> int:
    """Busca un agente por nombre; si no existe, lo crea (requiere rol).

    Esto es un fallback de seguridad: lo normal es poblar `agents` aparte
    con el catálogo completo (ver populate_agents_catalog), pero si llega
    un signature_agent que no estaba en el catálogo, no queremos que el
    seed reviente -- lo creamos con el rol que tengamos a mano, o None si
    no se pudo determinar (se puede corregir manualmente después).
    """
    cur.execute("SELECT id FROM agents WHERE name = %s", (agent_name,))
    row = cur.fetchone()
    if row:
        return row[0]

    cur.execute(
        "INSERT INTO agents (name, role) VALUES (%s, %s) RETURNING id",
        (agent_name, role),
    )
    print(f"    [AVISO] Agente '{agent_name}' no estaba en el catálogo, "
          f"se creó sobre la marcha con rol={role!r}. Revisa agent_roles.py "
          f"del repo de data-fetch para mantener el catálogo sincronizado.")
    return cur.fetchone()[0]


# Mapa agent -> role duplicado aquí en forma reducida SOLO como fallback
# para poblar la tabla `agents` de forma autocontenida sin depender del
# repo data-fetch en tiempo de seed. La fuente de verdad real es
# data-fetch/agent_roles.py; si agregas un agente nuevo allá, agrégalo
# también aquí para que el catálogo quede completo desde el inicio.
AGENT_ROLE_CATALOG = {
    "jett": "Duelist", "phoenix": "Duelist", "reyna": "Duelist", "raze": "Duelist",
    "yoru": "Duelist", "neon": "Duelist", "iso": "Duelist", "waylay": "Duelist",
    "brimstone": "Controller", "omen": "Controller", "viper": "Controller",
    "astra": "Controller", "harbor": "Controller", "clove": "Controller", "veto": "Controller",
    "sova": "Initiator", "breach": "Initiator", "skye": "Initiator", "kayo": "Initiator",
    "fade": "Initiator", "gekko": "Initiator", "tejo": "Initiator",
    "sage": "Sentinel", "cypher": "Sentinel", "killjoy": "Sentinel", "chamber": "Sentinel",
    "deadlock": "Sentinel", "vyse": "Sentinel", "miks": "Sentinel",
}


def populate_agent_catalog(cur):
    """Asegura que el catálogo completo de agentes exista, sin importar
    si algún jugador del dataset usa o no cada agente todavía."""
    for name, role in AGENT_ROLE_CATALOG.items():
        cur.execute(
            """
            INSERT INTO agents (name, role) VALUES (%s, %s)
            ON CONFLICT (name) DO NOTHING
            """,
            (name, role),
        )


def normalize_role(role) -> object:
    """El ENUM player_role en Postgres no acepta '' ni valores fuera de
    su set; None se mapea directo a NULL, que es válido (columna nullable)."""
    if not role:
        return None
    return role


def upsert_player(cur, player: dict, team_id: int) -> int:
    signature_agent_id = None
    if player.get("signature_agent"):
        signature_agent_id = get_or_create_agent_id(
            cur, player["signature_agent"], role=normalize_role(player.get("role"))
        )

    cur.execute(
        """
        INSERT INTO players (
            alias, real_name, country_code, current_team_id,
            role, role_confidence_pct, signature_agent_id,
            is_captain, avatar_url, source_player_id
        )
        VALUES (
            %(alias)s, %(real_name)s, %(country_code)s, %(current_team_id)s,
            %(role)s, %(role_confidence_pct)s, %(signature_agent_id)s,
            %(is_captain)s, %(avatar_url)s, %(source_player_id)s
        )
        ON CONFLICT (source_player_id) DO UPDATE SET
            alias = EXCLUDED.alias,
            real_name = EXCLUDED.real_name,
            country_code = EXCLUDED.country_code,
            current_team_id = EXCLUDED.current_team_id,
            role = EXCLUDED.role,
            role_confidence_pct = EXCLUDED.role_confidence_pct,
            signature_agent_id = EXCLUDED.signature_agent_id,
            is_captain = EXCLUDED.is_captain,
            avatar_url = EXCLUDED.avatar_url
        RETURNING id
        """,
        {
            "alias": player["alias"],
            "real_name": player.get("real_name") or None,
            "country_code": player.get("country") or None,
            "current_team_id": team_id,
            "role": normalize_role(player.get("role")),
            "role_confidence_pct": player.get("role_confidence_pct"),
            "signature_agent_id": signature_agent_id,
            "is_captain": bool(player.get("is_captain", False)),
            "avatar_url": player.get("avatar_url") or None,
            "source_player_id": player.get("source_player_id") or None,
        },
    )
    return cur.fetchone()[0]

def replace_past_teams(cur, player_id: int, past_teams: list):
    """Borra y re-inserta las filas de equipos pasados de un jugador.

    No se intenta resolver raw_team_name a un teams.id existente en este
    seed inicial (ver decisión del usuario: el ruido nombre+fecha pegado
    se deja crudo por ahora). team_id queda NULL; se puede resolver en una
    pasada de limpieza futura sin tocar este script.
    """
    cur.execute("DELETE FROM player_past_teams WHERE player_id = %s", (player_id,))
    if not past_teams:
        return
    rows = [
        (player_id, raw_name, idx)
        for idx, raw_name in enumerate(past_teams)
        if raw_name
    ]
    psycopg2.extras.execute_values(
        cur,
        "INSERT INTO player_past_teams (player_id, raw_team_name, sort_order) VALUES %s",
        rows,
    )


def replace_titles(cur, player_id: int, titles: list):
    """Borra y re-inserta los títulos de un jugador.

    Cada título llega como 'Nombre del evento (AÑO)' (ver
    fetch_dataset.extract_titles). Se separa el año al final entre
    paréntesis; si no matchea ese patrón, event_year queda NULL pero el
    raw_label siempre se preserva completo.
    """
    cur.execute("DELETE FROM player_titles WHERE player_id = %s", (player_id,))
    if not titles:
        return

    rows = []
    for raw_label in titles:
        if not raw_label:
            continue
        event_name, event_year = raw_label, None
        if raw_label.endswith(")") and "(" in raw_label:
            open_paren = raw_label.rfind("(")
            possible_year = raw_label[open_paren + 1:-1]
            if possible_year.isdigit() and len(possible_year) == 4:
                event_name = raw_label[:open_paren].strip()
                event_year = int(possible_year)
        rows.append((player_id, event_name, event_year, raw_label))

    psycopg2.extras.execute_values(
        cur,
        "INSERT INTO player_titles (player_id, event_name, event_year, raw_label) VALUES %s",
        rows,
    )


def seed(conn, teams_data: list):
    with conn.cursor() as cur:
        populate_agent_catalog(cur)

        total_teams = 0
        total_players = 0

        for team in teams_data:
            team_id = upsert_team(cur, team)
            total_teams += 1

            for player in team.get("players", []):
                player_id = upsert_player(cur, player, team_id)
                replace_past_teams(cur, player_id, player.get("past_teams", []))
                replace_titles(cur, player_id, player.get("titles", []))
                total_players += 1

        conn.commit()
        print(f"\nSeed completo: {total_teams} equipos, {total_players} jugadores procesados.")


def main():
    parser = argparse.ArgumentParser(description="Carga dataset.json a Postgres")
    parser.add_argument("--dataset", default="dataset.json", help="Ruta al dataset.json")
    parser.add_argument("--dsn", default=DEFAULT_DSN,
                         help="Connection string de Postgres (o usa la env var DATABASE_URL)")
    args = parser.parse_args()

    if not os.path.exists(args.dataset):
        print(f"ERROR: no se encontró '{args.dataset}'. Cópialo a esta carpeta o usa --dataset.")
        sys.exit(1)

    teams_data = load_dataset(args.dataset)
    print(f"Dataset cargado: {len(teams_data)} equipos encontrados en {args.dataset}\n")

    conn = psycopg2.connect(args.dsn)
    try:
        seed(conn, teams_data)
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


if __name__ == "__main__":
    main()
