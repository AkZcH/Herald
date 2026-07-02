-- config_versions: append-only after commit, never mutated
CREATE TABLE config_versions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key              TEXT NOT NULL,
    namespace        TEXT NOT NULL DEFAULT 'default',
    value            JSONB NOT NULL,
    previous_id      UUID REFERENCES config_versions(id),
    vector_clock     JSONB NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL CHECK (status IN (
                       'PENDING','PROPAGATING','COMMITTED','ROLLING_BACK','ROLLED_BACK'
                     )),
    rollback_status  TEXT,
    ttl_expires_at   TIMESTAMPTZ,
    targeting_labels JSONB,
    pushed_by        TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_config_versions_key_ns     ON config_versions(key, namespace);
CREATE INDEX idx_config_versions_status     ON config_versions(status);
CREATE INDEX idx_config_versions_created_at ON config_versions(created_at DESC);

-- raft_log: strictly append-only, never updated after write
CREATE TABLE raft_log (
    id                BIGSERIAL PRIMARY KEY,
    term              BIGINT NOT NULL,
    log_index         BIGINT NOT NULL UNIQUE,
    node_id           TEXT NOT NULL,
    config_version_id UUID REFERENCES config_versions(id),
    command           TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_raft_log_term_index ON raft_log(term, log_index);
