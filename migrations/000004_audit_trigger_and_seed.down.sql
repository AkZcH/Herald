DROP TRIGGER IF EXISTS enforce_audit_immutability ON audit_log;
DROP FUNCTION IF EXISTS audit_log_immutable();
DELETE FROM raft_log WHERE log_index = 0;
DELETE FROM config_versions WHERE id = '00000000-0000-0000-0000-000000000000';
