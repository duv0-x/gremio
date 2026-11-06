-- +goose Up
-- Plugins habilitados (ADR-012): un plugin está habilitado si tiene fila. config es un objeto JSON con los
-- campos que pide el plugin, secretos incluidos (el panel nunca los devuelve). status es el resumen o el error
-- del último chequeo. Fechas en el formato fijo 2006-01-02T15:04:05.000Z.
CREATE TABLE plugins (
    id         TEXT PRIMARY KEY,
    config     TEXT NOT NULL,
    status     TEXT NOT NULL,
    checked_at TEXT NOT NULL,
    enabled_at TEXT NOT NULL
) STRICT;
