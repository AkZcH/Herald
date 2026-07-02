-- saved targeting selectors (reusable label selector expressions)
CREATE TABLE targeting_rules (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    namespace  TEXT NOT NULL,
    name       TEXT NOT NULL,
    selector   JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_targeting_rules_namespace ON targeting_rules(namespace);

-- schema registry: type + constraint definitions per key+namespace
CREATE TABLE schema_registry (
    key         TEXT NOT NULL,
    namespace   TEXT NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('string','integer','float','boolean','enum')),
    constraints JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (key, namespace)
);

-- audit log: append-only, every write/rollback/schema change recorded here
CREATE TABLE audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type   TEXT NOT NULL,
    key          TEXT NOT NULL,
    namespace    TEXT NOT NULL,
    old_value    JSONB,
    new_value    JSONB,
    version_id   UUID REFERENCES config_versions(id),
    vector_clock JSONB,
    actor        TEXT NOT NULL,
    targeting    JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_namespace   ON audit_log(namespace);
CREATE INDEX idx_audit_log_key         ON audit_log(key, namespace);
CREATE INDEX idx_audit_log_created_at  ON audit_log(created_at DESC);
CREATE INDEX idx_audit_log_event_type  ON audit_log(event_type);
