-- Alert lifecycle extensions
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS acknowledged_by TEXT;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS acknowledged_comment TEXT;

-- Alert rule extensions
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS severity TEXT NOT NULL DEFAULT 'warning';
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS for_duration_seconds INT NOT NULL DEFAULT 0;
ALTER TABLE alert_rules ADD COLUMN IF NOT EXISTS cooldown_seconds INT NOT NULL DEFAULT 0;

-- Incident notes & comments
CREATE TABLE IF NOT EXISTS alert_comments (
    id TEXT PRIMARY KEY,
    alert_id TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    author_id TEXT NOT NULL,
    comment TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Notification channels
CREATE TABLE IF NOT EXISTS notification_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- 'webhook', 'slack_webhook'
    url TEXT NOT NULL,
    secret TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Notification deliveries log
CREATE TABLE IF NOT EXISTS notification_deliveries (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    alert_id TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'delivered', 'failed'
    attempts INT NOT NULL DEFAULT 0,
    response_status INT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

-- API tokens & RBAC
CREATE TABLE IF NOT EXISTS api_tokens (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL DEFAULT 'operator', -- 'viewer', 'operator', 'admin'
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ
);

-- Telemetry hourly summary rollup table
CREATE TABLE IF NOT EXISTS telemetry_hourly (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    bucket_start TIMESTAMPTZ NOT NULL,
    cpu_avg DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_min DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_max DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_avg DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_min DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_max DOUBLE PRECISION NOT NULL DEFAULT 0,
    disk_avg DOUBLE PRECISION NOT NULL DEFAULT 0,
    network_received BIGINT NOT NULL DEFAULT 0,
    network_sent BIGINT NOT NULL DEFAULT 0,
    sample_count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, bucket_start)
);

-- Performance Indexes
CREATE INDEX IF NOT EXISTS idx_alert_comments_alert ON alert_comments (alert_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_channel ON notification_deliveries (channel_id, status);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens (token_hash) WHERE enabled = TRUE;
CREATE INDEX IF NOT EXISTS idx_telemetry_hourly_agent_bucket ON telemetry_hourly (agent_id, bucket_start DESC);
