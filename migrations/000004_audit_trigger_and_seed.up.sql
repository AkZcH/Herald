-- enforce append-only on audit_log at the database level
CREATE OR REPLACE FUNCTION audit_log_immutable()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: UPDATE and DELETE are not permitted';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_audit_immutability
BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

-- seed default namespaces into schema_registry as documentation
-- (no actual rows needed, namespaces are implicit in config_versions)
-- seed a sentinel raft_log entry so log_index starts at 1
INSERT INTO config_versions (
    id, key, namespace, value, vector_clock, status, pushed_by
) VALUES (
    '00000000-0000-0000-0000-000000000000',
    '__sentinel__',
    '__system__',
    '"sentinel"',
    '{}',
    'COMMITTED',
    'system'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO raft_log (
    term, log_index, node_id, config_version_id, command
) VALUES (
    0, 0, 'system', '00000000-0000-0000-0000-000000000000', 'INIT'
) ON CONFLICT (log_index) DO NOTHING;
