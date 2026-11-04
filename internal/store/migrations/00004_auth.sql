-- +goose Up
-- Login del panel por enlace de un solo uso (ADR-009). Solo se guardan hashes SHA-256 de los tokens.
-- Las fechas usan el formato fijo 2006-01-02T15:04:05.000Z para que comparar texto sea comparar tiempo.
ALTER TABLE instance ADD COLUMN owner_email TEXT;

-- Enlaces enviados: solo vale el último (pedir uno nuevo invalida el anterior); las filas sirven además
-- para el límite de envíos.
CREATE TABLE login_links (
    id         INTEGER PRIMARY KEY,
    token_hash BLOB    NOT NULL,
    created_at TEXT    NOT NULL,
    used_at    TEXT
) STRICT;

CREATE TABLE sessions (
    token_hash BLOB PRIMARY KEY,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;
