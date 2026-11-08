-- +goose Up
-- Retos propios y ejecuciones del dojo (ADR-002 enmendado por ADR-013). Los retos del catálogo revisado viven
-- en el repo (internal/dojo/retos), no aquí. Los documentos son JSON con el formato de retos.Challenge.
-- Fechas en el formato fijo 2006-01-02T15:04:05.000Z.

-- Un reto propio: draft es el borrador editable (o generado por IA); publicarlo crea una versión inmutable.
CREATE TABLE challenges (
    id         TEXT PRIMARY KEY,
    draft      TEXT CHECK (draft IS NULL OR json_valid(draft)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
) STRICT;

-- Versiones publicadas: nunca se editan ni se borran (ADR-002 §3).
CREATE TABLE challenge_versions (
    challenge_id TEXT    NOT NULL REFERENCES challenges (id),
    version      INTEGER NOT NULL CHECK (version > 0),
    document     TEXT    NOT NULL CHECK (json_valid(document)),
    published_at TEXT    NOT NULL,
    PRIMARY KEY (challenge_id, version)
) STRICT;

-- La ejecución guarda la copia del enunciado que se tomó (snapshot), así la DoD no cambia sin consentimiento.
-- challenge es el id del reto original (raíz del linaje basedOn): "catalog:<slug>" o la URL del reto propio.
-- La entrega (repo, commit fijado, demo, informe) va en la misma fila: se reemplaza mientras no esté terminada.
-- verification es el JSON del último resultado de los checks. Al publicar entra al libro challengeStarted con la
-- fecha de inicio (ADR-014) y, al terminar con verificación aprobada, challengeCompleted; sus hashes quedan aquí
-- para retirarlos.
CREATE TABLE executions (
    id                TEXT    PRIMARY KEY,
    challenge         TEXT    NOT NULL,
    challenge_version INTEGER NOT NULL,
    snapshot          TEXT    NOT NULL CHECK (json_valid(snapshot)),
    status            TEXT    NOT NULL DEFAULT 'draft'
                      CHECK (status IN ('draft', 'in_progress', 'done', 'abandoned')),
    repo              TEXT    NOT NULL DEFAULT '',
    commit_sha        TEXT    NOT NULL DEFAULT '',
    demo              TEXT    NOT NULL DEFAULT '',
    report            TEXT    NOT NULL DEFAULT '',
    notes             TEXT    NOT NULL DEFAULT '',
    verification      TEXT    CHECK (verification IS NULL OR json_valid(verification)),
    started_at        TEXT    NOT NULL,
    completed_at      TEXT,
    published_at      TEXT,
    retired_at        TEXT,
    started_hash      TEXT,
    completed_hash    TEXT,
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL
) STRICT;

CREATE INDEX executions_challenge ON executions (challenge);
