# ── Stage 1: compile Apache AGE for PostgreSQL 16 ───────────────────────────
FROM postgres:16-bookworm AS age-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates git libreadline-dev zlib1g-dev flex bison \
    postgresql-server-dev-16 \
    && rm -rf /var/lib/apt/lists/*

# pg_config must be on PATH for the AGE Makefile
ENV PATH="/usr/lib/postgresql/16/bin:$PATH"

ARG AGE_TAG=PG16/v1.6.0-rc0
RUN git clone --depth 1 --branch "${AGE_TAG}" \
        https://github.com/apache/age.git /age \
    && cd /age && make && make install

# ── Stage 2: pgvector/pgvector:pg16 + AGE extension files ───────────────────
FROM pgvector/pgvector:pg16

# Copy AGE shared library
COPY --from=age-builder \
    /usr/lib/postgresql/16/lib/age.so \
    /usr/lib/postgresql/16/lib/

# Merge AGE extension SQL + control files into the existing extension directory.
# COPY is additive — it does NOT delete pgvector files already present.
COPY --from=age-builder \
    /usr/share/postgresql/16/extension/ \
    /usr/share/postgresql/16/extension/

# Create the pgvector (and AGE) extensions on first initialisation, so the
# MemPalace server can register the vector type on its very first connection.
# Without this a brand-new database has no `vector` type and the server fails to
# boot. Runs only when the data directory is empty (standard postgres image
# behaviour); a no-op on an already-initialised volume.
COPY init-extensions.sh /docker-entrypoint-initdb.d/10-init-extensions.sh
RUN chmod +x /docker-entrypoint-initdb.d/10-init-extensions.sh
