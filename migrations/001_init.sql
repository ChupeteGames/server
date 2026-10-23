-- ============================================================================
-- Esquema Inicial PostgreSQL 17 — Shooter de Predicción en Primera Persona
-- Basado en el GDD de @Santiago (Oct 2026) y la arquitectura Go del equipo.
-- ============================================================================
-- NOTA ARQUITECTÓNICA (DDD / Hexagonal):
-- Este esquema modela la persistencia relacional (Adaptador Secundario de Storage)
-- que da soporte a los Bounded Contexts del servidor:
--   1. Auth & Players  -> users, player_profiles
--   2. Economy & Store -> cosmetic_catalog, wallets, player_cosmetics, economy_transactions
--   3. Matchmaking     -> matches, match_tickets
--   4. Matches (Stats) -> match_rounds, match_player_stats
--
-- El script está diseñado para ser IDEMPOTENTE (CREATE TABLE IF NOT EXISTS /
-- CREATE INDEX IF NOT EXISTS / ON CONFLICT DO NOTHING), de modo que pueda
-- ejecutarse automáticamente al inicializar el contenedor Docker o manualmente
-- sin romper datos existentes.
-- ============================================================================

-- Extensión necesaria para generar UUIDs v4 criptográficamente seguros (gen_random_uuid)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ----------------------------------------------------------------------------
-- 1. Usuarios y Cuentas (Bounded Context: auth)
-- ----------------------------------------------------------------------------
-- Almacena las credenciales de acceso. Por seguridad, nunca se guarda la
-- contraseña en texto plano, únicamente su hash (bcrypt).
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------------------
-- 2. Catálogo de Cosméticos (Bounded Context: economy)
-- ----------------------------------------------------------------------------
-- Define todos los ítems disponibles en el juego: Skins estilo Fall Guys ('skin')
-- y Efectos de Explosión Sangrienta al morir ('death_effect').
-- Se crea antes de player_profiles porque el perfil referencia los cosméticos equipados.
CREATE TABLE IF NOT EXISTS cosmetic_catalog (
    code TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('skin', 'death_effect')),
    price_coins BIGINT NOT NULL CHECK (price_coins >= 0),
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------------------
-- 3. Perfil del Jugador, Progresión, Ranking (MMR) y Loadout (Bounded Context: players)
-- ----------------------------------------------------------------------------
-- Relación 1:1 con users. Concentra el estado competitivo del jugador (MMR inicial 1000),
-- sus estadísticas históricas acumuladas y los cosméticos que lleva equipados a la partida.
CREATE TABLE IF NOT EXISTS player_profiles (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    level INT NOT NULL DEFAULT 1 CHECK (level >= 1),
    xp BIGINT NOT NULL DEFAULT 0 CHECK (xp >= 0),
    mmr INT NOT NULL DEFAULT 1000 CHECK (mmr >= 0),
    equipped_skin_code TEXT NOT NULL DEFAULT 'bean_default' REFERENCES cosmetic_catalog(code),
    equipped_death_effect_code TEXT NOT NULL DEFAULT 'gore_classic' REFERENCES cosmetic_catalog(code),
    matches_played INT NOT NULL DEFAULT 0 CHECK (matches_played >= 0),
    matches_won INT NOT NULL DEFAULT 0 CHECK (matches_won >= 0),
    matches_drawn INT NOT NULL DEFAULT 0 CHECK (matches_drawn >= 0),
    total_kills INT NOT NULL DEFAULT 0 CHECK (total_kills >= 0),
    total_deaths INT NOT NULL DEFAULT 0 CHECK (total_deaths >= 0),
    mutual_kills INT NOT NULL DEFAULT 0 CHECK (mutual_kills >= 0),
    duels_won INT NOT NULL DEFAULT 0 CHECK (duels_won >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Índice descendente por MMR para optimizar consultas de Leaderboard y emparejamiento
CREATE INDEX IF NOT EXISTS idx_player_profiles_mmr ON player_profiles(mmr DESC);

-- ----------------------------------------------------------------------------
-- 4. Billetera del Jugador (Bounded Context: economy)
-- ----------------------------------------------------------------------------
-- Mantiene el saldo actual de monedas del jugador.
-- La restricción CHECK (balance >= 0) actúa como invariante de base de datos
-- para impedir saldos negativos incluso ante condiciones de carrera.
CREATE TABLE IF NOT EXISTS wallets (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------------------
-- 5. Inventario de Cosméticos Desbloqueados por Jugador (Bounded Context: economy)
-- ----------------------------------------------------------------------------
-- Tabla intermedia (N:M entre users y cosmetic_catalog) que registra qué skins
-- o efectos de muerte posee cada jugador. La PK compuesta evita compras duplicadas.
CREATE TABLE IF NOT EXISTS player_cosmetics (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cosmetic_code TEXT NOT NULL REFERENCES cosmetic_catalog(code),
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, cosmetic_code)
);

-- ----------------------------------------------------------------------------
-- 6. Libro Contable de Transacciones Económicas (Bounded Context: economy)
-- ----------------------------------------------------------------------------
-- Registro inmutable (ledger) de cada variación de monedas (premios de partida,
-- compras en tienda o saldo inicial).
-- CLAVE DE IDEMPOTENCIA: operation_id (UUID) impide que un reintento de red
-- cobre o acredite monedas dos veces por la misma operación.
CREATE TABLE IF NOT EXISTS economy_transactions (
    operation_id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    transaction_type TEXT NOT NULL CHECK (transaction_type IN ('match_reward', 'store_purchase', 'initial_grant')),
    coins_delta BIGINT NOT NULL,
    reference_code TEXT NULL, -- ej. match_id o cosmetic_code
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_economy_tx_user_created ON economy_transactions(user_id, created_at DESC);

-- ----------------------------------------------------------------------------
-- 7. Partidas de 2v2 a 5v5 (Bounded Context: matchmaking / matches)
-- ----------------------------------------------------------------------------
-- Representa el ciclo de vida de una partida en el Unreal Dedicated Server.
-- report_operation_id es UNIQUE para garantizar idempotencia cuando el servidor
-- dedicado de Unreal reporta el cierre de partida (ReportMatchResult vía gRPC).
CREATE TABLE IF NOT EXISTS matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mode TEXT NOT NULL CHECK (mode IN ('2v2', '3v3', '4v4', '5v5')),
    status TEXT NOT NULL DEFAULT 'created' CHECK (status IN ('created', 'in_progress', 'completed', 'cancelled')),
    outcome TEXT NULL CHECK (outcome IN ('TEAM_A_WIN', 'TEAM_B_WIN', 'DRAW_MUTUAL_ELIMINATION', 'DRAW_STALEMATE_TIMER')),
    total_rounds INT NOT NULL DEFAULT 0 CHECK (total_rounds >= 0),
    ended_in_duel BOOLEAN NOT NULL DEFAULT false,
    duration_seconds INT NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
    server_host TEXT NOT NULL DEFAULT '127.0.0.1',
    server_port INT NOT NULL DEFAULT 7777,
    report_operation_id UUID NULL UNIQUE, -- Evita procesar dos veces el cierre de la misma partida
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ NULL,
    ended_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_matches_status ON matches(status);

-- ----------------------------------------------------------------------------
-- 8. Tickets de Acceso al Unreal Dedicated Server (Bounded Context: matchmaking)
-- ----------------------------------------------------------------------------
-- Emitidos por Go cuando el matchmaking empareja a los jugadores.
-- El cliente Unreal envía este ticket_token al conectarse al Dedicated Server,
-- y Unreal lo valida contra Go (ValidateJoinTicket en gRPC) para asignarle equipo.
CREATE TABLE IF NOT EXISTS match_tickets (
    ticket_token TEXT PRIMARY KEY,
    match_id UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_team TEXT NOT NULL CHECK (assigned_team IN ('TEAM_A', 'TEAM_B')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (match_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_match_tickets_user ON match_tickets(user_id);

-- ----------------------------------------------------------------------------
-- 9. Historial Ronda por Ronda de cada Partida (Bounded Context: matches)
-- ----------------------------------------------------------------------------
-- Desglose táctico de cada ronda jugada: bajas, uso de Rondas de Gracia (cuando
-- un jugador no se expone al 70% o no dispara), muertes por penalización de gracia,
-- activación del timer de estancamiento (3 min) y transición a Duelo 1v1.
CREATE TABLE IF NOT EXISTS match_rounds (
    match_id UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    round_number INT NOT NULL CHECK (round_number >= 1),
    kills_in_round INT NOT NULL DEFAULT 0 CHECK (kills_in_round >= 0),
    grace_rounds_consumed INT NOT NULL DEFAULT 0 CHECK (grace_rounds_consumed >= 0),
    grace_penalty_deaths INT NOT NULL DEFAULT 0 CHECK (grace_penalty_deaths >= 0),
    stalemate_timer_active BOOLEAN NOT NULL DEFAULT false,
    triggered_duel_1v1 BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (match_id, round_number)
);

-- ----------------------------------------------------------------------------
-- 10. Estadísticas Detalladas por Jugador en cada Partida (Bounded Context: matches)
-- ----------------------------------------------------------------------------
-- Registra el rendimiento individual al finalizar la partida, incluyendo puntería
-- hacia la Pared central (valid_wall_shots vs wasted_shots), muertes mutuas y
-- las recompensas otorgadas (mmr_delta, coins_earned, xp_earned).
CREATE TABLE IF NOT EXISTS match_player_stats (
    match_id UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team TEXT NOT NULL CHECK (team IN ('TEAM_A', 'TEAM_B')),
    kills INT NOT NULL DEFAULT 0 CHECK (kills >= 0),
    deaths INT NOT NULL DEFAULT 0 CHECK (deaths >= 0),
    mutual_kills INT NOT NULL DEFAULT 0 CHECK (mutual_kills >= 0),
    shots_fired INT NOT NULL DEFAULT 0 CHECK (shots_fired >= 0),
    valid_wall_shots INT NOT NULL DEFAULT 0 CHECK (valid_wall_shots >= 0),
    wasted_shots INT NOT NULL DEFAULT 0 CHECK (wasted_shots >= 0),
    grace_round_used BOOLEAN NOT NULL DEFAULT false,
    died_by_grace_penalty BOOLEAN NOT NULL DEFAULT false,
    won_1v1_duel BOOLEAN NOT NULL DEFAULT false,
    mmr_delta INT NOT NULL DEFAULT 0,
    coins_earned BIGINT NOT NULL DEFAULT 0 CHECK (coins_earned >= 0),
    xp_earned BIGINT NOT NULL DEFAULT 0 CHECK (xp_earned >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (match_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_match_player_stats_user ON match_player_stats(user_id, created_at DESC);

-- ============================================================================
-- Datos Semilla Iniciales (Catálogo de Skins y Efectos de Explosión Sangrienta)
-- ============================================================================
-- Se insertan los 7 cosméticos base del MVP (4 skins y 3 efectos de muerte).
-- 'bean_default' y 'gore_classic' son gratuitos (price_coins = 0, is_default = true)
-- y funcionan como valores por defecto en player_profiles.
-- ON CONFLICT (code) DO NOTHING asegura que re-ejecutar el script no falle.
INSERT INTO cosmetic_catalog (code, name, category, price_coins, is_default) VALUES
    ('bean_default', 'Poroto Clásico (Azul/Rojo)', 'skin', 0, true),
    ('bean_knight', 'Caballero de Goma', 'skin', 200, false),
    ('bean_hotdog', 'Pancho Táctico', 'skin', 350, false),
    ('bean_samurai', 'Samurái Pastel', 'skin', 500, false),
    ('gore_classic', 'Explosión Carnicera Clásica', 'death_effect', 0, true),
    ('gore_fountain', 'Geiser de Sangre y Huesos', 'death_effect', 300, false),
    ('gore_meat_confetti', 'Confeti de Vísceras', 'death_effect', 450, false)
ON CONFLICT (code) DO NOTHING;
