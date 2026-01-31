-- CloudScan Orchestrator - Rollback Initial Schema

DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS findings CASCADE;
DROP TABLE IF EXISTS scans CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS organizations CASCADE;

DROP FUNCTION IF EXISTS update_updated_at_column CASCADE;

DROP EXTENSION IF EXISTS "uuid-ossp";