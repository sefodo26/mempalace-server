#!/bin/bash
# Runs once, on first initialisation of an empty data directory (the standard
# postgres image executes everything in /docker-entrypoint-initdb.d/ then).
#
# Why this is needed: the MemPalace server registers the pgvector type on every
# new pool connection (AfterConnect -> RegisterTypes) *before* it runs its own
# schema provisioning. On a brand-new database the `vector` type does not exist
# yet, so the very first connection fails with "vector type not found in the
# database" and the server never boots. Creating the extension here, at initdb
# time, breaks that chicken-and-egg — the type exists before the app connects.
#
# AGE is created best-effort: it needs shared_preload_libraries=age, which the
# deployment passes as a runtime arg. If it isn't active during initdb we don't
# fail the whole bootstrap — vector is the only hard requirement.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-'EOSQL'
    CREATE EXTENSION IF NOT EXISTS vector;
EOSQL

if psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        -c "CREATE EXTENSION IF NOT EXISTS age;" 2>/dev/null; then
    echo "init-extensions: created vector + age"
else
    echo "init-extensions: created vector; age deferred (needs shared_preload_libraries=age at runtime)"
fi
