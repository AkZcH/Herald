-- per-agent ack state per config version
CREATE TABLE ack_states (
    version_id UUID NOT NULL REFERENCES config_versions(id),
    agent_id   TEXT NOT NULL,
    acked_at   TIMESTAMPTZ,
    status     TEXT NOT NULL CHECK (status IN ('PENDING','ACKED','FAILED')),
    PRIMARY KEY (version_id, agent_id)
);

CREATE INDEX idx_ack_states_version_id ON ack_states(version_id);
CREATE INDEX idx_ack_states_status     ON ack_states(status);

-- failed deliveries: written when agent does not ack within SLA window
CREATE TABLE delivery_failures (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID NOT NULL REFERENCES config_versions(id),
    agent_id   TEXT NOT NULL,
    reason     TEXT,
    failed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_delivery_failures_version_id ON delivery_failures(version_id);
CREATE INDEX idx_delivery_failures_agent_id   ON delivery_failures(agent_id);

-- agent registry: updated on every heartbeat
CREATE TABLE agent_registry (
    agent_id      TEXT PRIMARY KEY,
    namespace     TEXT NOT NULL,
    labels        JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL CHECK (status IN ('HEALTHY','STALE','DEAD')),
    last_seen     TIMESTAMPTZ NOT NULL,
    version_id    UUID REFERENCES config_versions(id),
    config_hash   TEXT,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_registry_namespace ON agent_registry(namespace);
CREATE INDEX idx_agent_registry_status    ON agent_registry(status);
CREATE INDEX idx_agent_registry_last_seen ON agent_registry(last_seen);
