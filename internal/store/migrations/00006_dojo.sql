-- +goose Up
-- Libro de eventos del dojo (ADR-013 §6). Append-only salvo el retiro: retirar reemplaza body por una tombstone
-- (reto, fecha y hash) y agrega un evento "retired". seq es el orden de la cadena; hash es el sha256 de body
-- (que incluye prev), y prev el hash del evento anterior ('' en el primero). El Entrenamiento no se guarda.
CREATE TABLE dojo_events (
    seq       INTEGER PRIMARY KEY,
    hash      TEXT NOT NULL UNIQUE,
    prev      TEXT NOT NULL,
    type      TEXT NOT NULL,
    at        TEXT NOT NULL,
    challenge TEXT NOT NULL,
    body      TEXT NOT NULL
) STRICT;

CREATE INDEX dojo_events_challenge ON dojo_events (challenge);

