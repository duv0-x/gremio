-- +goose Up
-- Propiedades de Gremio de un Note generado por Actividad (ADR-014 §2): el objeto JSON con gremio:event,
-- gremio:ref, gremio:rules y gremio:head, tal cual se publica. Vacío en un Note sin evento.
ALTER TABLE notes ADD COLUMN gremio TEXT NOT NULL DEFAULT '';
