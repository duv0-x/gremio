-- +goose Up
-- host y account_domain se fijan en el primer `serve` y no cambian nunca (ADR-001).
ALTER TABLE instance ADD COLUMN host TEXT;
ALTER TABLE instance ADD COLUMN account_domain TEXT;

-- El perfil es un documento JSON Resume (ADR-003, ADR-006).
CREATE TABLE profile (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    document   TEXT    NOT NULL CHECK (json_valid(document)),
    updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

-- Instancias creadas antes de esta migración: perfil mínimo con el usuario como nombre.
INSERT INTO profile (id, document)
SELECT 1, json_object('basics', json_object('name', username)) FROM instance;

CREATE TABLE followers (
    actor_iri    TEXT PRIMARY KEY,
    inbox        TEXT NOT NULL,
    shared_inbox TEXT,
    follow_iri   TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;
