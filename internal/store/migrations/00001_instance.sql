-- +goose Up
-- Una instancia es de una sola persona (ADR-001): la fila id = 1 es su dueño y su par de llaves.
CREATE TABLE instance (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    username        TEXT    NOT NULL,
    private_key_pem TEXT    NOT NULL,
    public_key_pem  TEXT    NOT NULL,
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;
