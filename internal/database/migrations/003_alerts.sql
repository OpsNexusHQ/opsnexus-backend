CREATE TABLE IF NOT EXISTS alert_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    duration_seconds INT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY,
    rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'firing',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    last_value DOUBLE PRECISION NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_agent_status ON alerts (agent_id, status);
CREATE INDEX IF NOT EXISTS idx_alerts_rule_status ON alerts (rule_id, status);
CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules (enabled);

-- Seed default alert rules
INSERT INTO alert_rules (id, name, metric, operator, threshold, duration_seconds, enabled)
VALUES
    ('rule-cpu-high', 'High CPU Usage', 'cpu_usage', '>', 80.0, 300, true),
    ('rule-mem-high', 'High Memory Usage', 'memory_usage', '>', 85.0, 300, true),
    ('rule-disk-high', 'High Disk Usage', 'disk_usage', '>', 90.0, 0, true),
    ('rule-agent-offline', 'Agent Offline', 'agent_offline', '==', 1.0, 0, true)
ON CONFLICT (id) DO NOTHING;
