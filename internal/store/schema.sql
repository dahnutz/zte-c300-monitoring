-- Additive schema for durable ONU samples and derived event tables.
-- Redis remains the hot cache; this database is history, identities and
-- poller-derived events. CREATE TABLE IF NOT EXISTS never drops
-- onu_samples or collection_runs. Versioned ALTERs in migrate() only add
-- columns/tables and backfill. This file is the empty-volume shape;
-- existing deployments reach it through schema_migrations (currently v3).

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version integer PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
    id text PRIMARY KEY,
    vendor text NOT NULL,
    family text NOT NULL,
    role text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS onus (
    device_id text NOT NULL REFERENCES devices (id),
    serial text NOT NULL,
    name text NOT NULL DEFAULT '',
    onu_type text NOT NULL DEFAULT '',
    last_board integer,
    last_pon integer,
    last_onu_id integer,
    last_status text NOT NULL DEFAULT '',
    previous_status text NOT NULL DEFAULT '',
    status_changed_at timestamptz,
    expected_eth_ports integer,
    last_seen_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, serial)
);

-- MAC/VLAN last-seen lands in a later stage. ONU identity is serial.
CREATE TABLE IF NOT EXISTS onu_samples (
    time timestamptz NOT NULL,
    device_id text NOT NULL,
    serial text NOT NULL DEFAULT '',
    board integer NOT NULL,
    pon integer NOT NULL,
    onu_id integer NOT NULL,
    name text NOT NULL DEFAULT '',
    onu_type text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT '',
    rx_power double precision,
    tx_power double precision,
    eth_status text NOT NULL DEFAULT '',
    eth_link_state text NOT NULL DEFAULT '',
    eth_admin_state text NOT NULL DEFAULT '',
    eth_speed_mbps integer,
    eth_ports jsonb
);

CREATE TABLE IF NOT EXISTS collection_runs (
    id bigserial PRIMARY KEY,
    device_id text NOT NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    status text NOT NULL,
    pons_ok integer NOT NULL DEFAULT 0,
    pons_error integer NOT NULL DEFAULT 0,
    onus_sampled integer NOT NULL DEFAULT 0,
    duration_ms integer,
    error text
);

-- Status and UNI change log. Derived from onu_samples; samples stay the source.
CREATE TABLE IF NOT EXISTS onu_status_events (
    time timestamptz NOT NULL,
    device_id text NOT NULL,
    serial text NOT NULL,
    board integer,
    pon integer,
    onu_id integer,
    status text NOT NULL,
    previous_status text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT 'poller'
);

CREATE TABLE IF NOT EXISTS onu_eth_events (
    time timestamptz NOT NULL,
    device_id text NOT NULL,
    serial text NOT NULL,
    board integer,
    pon integer,
    onu_id integer,
    port integer NOT NULL,
    link_state text NOT NULL DEFAULT '',
    admin_state text NOT NULL DEFAULT '',
    previous_link text NOT NULL DEFAULT '',
    previous_admin text NOT NULL DEFAULT '',
    speed_mbps integer,
    source text NOT NULL DEFAULT 'poller'
);

CREATE TABLE IF NOT EXISTS onu_eth_state (
    device_id text NOT NULL,
    serial text NOT NULL,
    port integer NOT NULL,
    admin_state text NOT NULL DEFAULT '',
    link_state text NOT NULL DEFAULT '',
    speed_mbps integer,
    duplex text NOT NULL DEFAULT '',
    last_changed_at timestamptz,
    last_seen_at timestamptz,
    PRIMARY KEY (device_id, serial, port)
);

-- Devices seen by the OLT but not provisioned. Never written by deleting samples.
CREATE TABLE IF NOT EXISTS onu_unauth (
    device_id text NOT NULL,
    serial text NOT NULL,
    board integer,
    pon integer,
    onu_type text NOT NULL DEFAULT '',
    discovery_status text NOT NULL DEFAULT '',
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    gone_at timestamptz,
    PRIMARY KEY (device_id, serial)
);

CREATE TABLE IF NOT EXISTS onu_unauth_samples (
    time timestamptz NOT NULL,
    device_id text NOT NULL,
    serial text NOT NULL,
    board integer,
    pon integer,
    onu_type text NOT NULL DEFAULT '',
    discovery_status text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS onu_unauth_discovery (
    device_id text PRIMARY KEY,
    status text NOT NULL,
    oid text NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    observed_at timestamptz NOT NULL,
    count integer NOT NULL DEFAULT 0
);
