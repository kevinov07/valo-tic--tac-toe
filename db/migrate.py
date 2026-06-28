r"""
migrate.py - Ejecuta schema.sql + seed_to_postgres.py contra Neon.

Uso:
    cd db
    ..\db\venv\Scripts\python migrate.py "postgresql://usuario:pass@ep-xxxx.neon.tech/valo-ttt?sslmode=require"
"""
import os
import sys
import subprocess
import psycopg2

dsn = sys.argv[1] if len(sys.argv) > 1 else input("DATABASE_URL: ")

# 1. Schema
with open("schema.sql", encoding="utf-8") as f:
    schema_sql = f.read()

conn = psycopg2.connect(dsn)
conn.autocommit = True
with conn.cursor() as cur:
    cur.execute(schema_sql)
conn.close()
print("✓ Schema aplicado")

# 2. Migrations
migrations_dir = os.path.join(os.path.dirname(__file__), "migrations")
if os.path.isdir(migrations_dir):
    migrations = sorted(os.listdir(migrations_dir))
    if migrations:
        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        with conn.cursor() as cur:
            for m in migrations:
                if m.endswith(".sql"):
                    path = os.path.join(migrations_dir, m)
                    with open(path, encoding="utf-8") as f:
                        cur.execute(f.read())
                    print(f"✓ Migración {m} aplicada")
        conn.close()

# 2. Seed
dataset_path = os.path.abspath(
    r"..\..\valo-tic-tac-toe-data-fetch\dataset.json"
)
venv_python = os.path.join(os.path.dirname(__file__), "venv", "Scripts", "python.exe")
result = subprocess.run(
    [venv_python, "seed_to_postgres.py", "--dsn", dsn, "--dataset", dataset_path],
    cwd=os.path.dirname(__file__),
)
sys.exit(result.returncode)
