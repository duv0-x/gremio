-- +goose Up
-- Publicaciones propias; el outbox y /notes/{id} se construyen desde aquí.
CREATE TABLE notes (
    id           TEXT PRIMARY KEY,
    content      TEXT NOT NULL,
    published_at TEXT NOT NULL
) STRICT;

-- Entregas salientes pendientes (ADR-001). activity es el JSON ya serializado; se firma en cada intento.
-- Las fechas usan un formato fijo (2006-01-02T15:04:05.000Z) para que comparar texto sea comparar tiempo.
CREATE TABLE delivery_queue (
    id              INTEGER PRIMARY KEY,
    inbox           TEXT    NOT NULL,
    activity        TEXT    NOT NULL CHECK (json_valid(activity)),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT    NOT NULL,
    last_error      TEXT,
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE INDEX delivery_queue_next_attempt ON delivery_queue (next_attempt_at);
