-- Migration 001: Add logo column to teams table
-- Creada: 2026-06-28

ALTER TABLE teams ADD COLUMN IF NOT EXISTS logo TEXT;

