--liquibase formatted sql

--changeset cloudscan:1 labels:v1.0.0 context:schema splitStatements:false
--comment: CloudScan Orchestrator - Base Tables

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Organizations (Multi-tenant)
-- =============================================================================
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_organizations_slug ON organizations(slug);
CREATE INDEX idx_organizations_active ON organizations(is_active) WHERE is_active = true;

-- =============================================================================
-- Projects
-- =============================================================================
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    repository_url TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT 'main',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(organization_id, slug)
);

CREATE INDEX idx_projects_org ON projects(organization_id);
CREATE INDEX idx_projects_active ON projects(is_active) WHERE is_active = true;

-- =============================================================================
-- Scans (Partitioned by created_at for performance)
-- =============================================================================
CREATE TABLE scans (
    id UUID NOT NULL,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    scan_types TEXT[] NOT NULL,

    -- Source code information
    repository_url TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    source_archive_key TEXT,

    -- Kubernetes job information
    job_name TEXT,
    job_namespace TEXT,

    -- Results
    findings_count INT NOT NULL DEFAULT 0,
    critical_count INT NOT NULL DEFAULT 0,
    high_count INT NOT NULL DEFAULT 0,
    medium_count INT NOT NULL DEFAULT 0,
    low_count INT NOT NULL DEFAULT 0,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,

    -- Audit
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create partitions for current and future months
CREATE TABLE scans_2025_01 PARTITION OF scans
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE scans_2025_02 PARTITION OF scans
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

CREATE TABLE scans_2025_03 PARTITION OF scans
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');

CREATE TABLE scans_2025_04 PARTITION OF scans
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');

CREATE TABLE scans_2025_05 PARTITION OF scans
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');


--changeset cloudscan:2 labels:v1.0.0 context:schema splitStatements:false
--comment: CloudScan Orchestrator - Scans Table

CREATE TABLE scans_2025_06 PARTITION OF scans
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');

-- Indexes on partitioned table
CREATE INDEX idx_scans_org ON scans(organization_id, created_at DESC);
CREATE INDEX idx_scans_project ON scans(project_id, created_at DESC);
CREATE INDEX idx_scans_user ON scans(user_id, created_at DESC);
CREATE INDEX idx_scans_status ON scans(status, created_at DESC);
CREATE INDEX idx_scans_job_name ON scans(job_name);

-- =============================================================================
-- Findings
-- =============================================================================

--changeset cloudscan:3 labels:v1.0.0 context:schema splitStatements:false
--comment: CloudScan Orchestrator - Findings and Audit Tables

CREATE TABLE findings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id UUID NOT NULL,

    -- Scanner information
    scan_type TEXT NOT NULL CHECK (scan_type IN ('sast', 'sca', 'secrets', 'license')),
    tool_name TEXT NOT NULL,
    tool_version TEXT,

    -- Finding details
    title TEXT NOT NULL,
    description TEXT,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low', 'info')),

    -- Location in code
    file_path TEXT,
    start_line INT,
    end_line INT,
    start_column INT,
    end_column INT,
    code_snippet TEXT,

    -- Vulnerability details (SAST/SCA)
    rule_id TEXT,
    cwe_id TEXT,
    cve_id TEXT,
    cvss_score DECIMAL(3,1),
    cvss_vector TEXT,

    -- Dependency information (SCA)
    package_name TEXT,
    package_version TEXT,
    fixed_version TEXT,

    -- License information
    license_name TEXT,
    license_type TEXT,

    -- Remediation
    remediation TEXT,
    "references" TEXT[],

    -- Metadata
    raw_output JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_findings_scan ON findings(scan_id);
CREATE INDEX idx_findings_severity ON findings(severity, scan_id);
CREATE INDEX idx_findings_type ON findings(scan_type, scan_id);
CREATE INDEX idx_findings_cve ON findings(cve_id) WHERE cve_id IS NOT NULL;

-- =============================================================================
-- Audit Log
-- =============================================================================
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id UUID,
    metadata JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_org ON audit_logs(organization_id, created_at DESC);
CREATE INDEX idx_audit_logs_user ON audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);

-- =============================================================================

--changeset cloudscan:4 labels:v1.0.0 context:schema splitStatements:false
--comment: CloudScan Orchestrator - Functions and Triggers

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $BODY$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$BODY$
language 'plpgsql';

-- Triggers to auto-update updated_at
CREATE TRIGGER update_organizations_updated_at BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

--changeset cloudscan:5 labels:v1.0.0 context:data
--comment: Insert sample data for development

-- Sample organization
INSERT INTO organizations (id, name, slug) VALUES
    ('550e8400-e29b-41d4-a716-446655440000', 'Demo Organization', 'demo-org');

-- Sample project
INSERT INTO projects (id, organization_id, name, slug, repository_url, default_branch) VALUES
    ('550e8400-e29b-41d4-a716-446655440001', '550e8400-e29b-41d4-a716-446655440000', 'Demo Project', 'demo-project', 'https://github.com/demo/repo', 'main');

--rollback DELETE FROM projects WHERE id = '550e8400-e29b-41d4-a716-446655440001';
--rollback DELETE FROM organizations WHERE id = '550e8400-e29b-41d4-a716-446655440000';
