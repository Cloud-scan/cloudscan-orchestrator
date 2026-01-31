package domain

import (
	"time"

	"github.com/google/uuid"
)

// Severity represents the severity level of a security finding
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// Finding represents a security vulnerability or issue found during a scan
type Finding struct {
	ID     uuid.UUID `json:"id" db:"id"`
	ScanID uuid.UUID `json:"scan_id" db:"scan_id"`

	// Scanner information
	ScanType    ScanType `json:"scan_type" db:"scan_type"` // Which scanner found this
	ToolName    string   `json:"tool_name" db:"tool_name"` // e.g., "semgrep", "trivy"
	ToolVersion string   `json:"tool_version" db:"tool_version"`

	// Finding details
	Title       string   `json:"title" db:"title"`
	Description string   `json:"description" db:"description"`
	Severity    Severity `json:"severity" db:"severity"`

	// Location in code
	FilePath    string `json:"file_path" db:"file_path"`
	StartLine   int    `json:"start_line" db:"start_line"`
	EndLine     int    `json:"end_line" db:"end_line"`
	StartColumn int    `json:"start_column" db:"start_column"`
	EndColumn   int    `json:"end_column" db:"end_column"`
	CodeSnippet string `json:"code_snippet" db:"code_snippet"`

	// Vulnerability details (for SAST/SCA)
	RuleID      string `json:"rule_id" db:"rule_id"`           // CWE, CVE, or scanner rule ID
	CWEID       string `json:"cwe_id" db:"cwe_id"`             // Common Weakness Enumeration
	CVEID       string `json:"cve_id" db:"cve_id"`             // Common Vulnerabilities and Exposures
	CVSSScore   float64 `json:"cvss_score" db:"cvss_score"`    // CVSS v3 score
	CVSSVector  string  `json:"cvss_vector" db:"cvss_vector"`  // CVSS vector string

	// Dependency information (for SCA)
	PackageName    string `json:"package_name" db:"package_name"`
	PackageVersion string `json:"package_version" db:"package_version"`
	FixedVersion   string `json:"fixed_version" db:"fixed_version"`

	// License information (for License scans)
	LicenseName string `json:"license_name" db:"license_name"`
	LicenseType string `json:"license_type" db:"license_type"` // permissive, copyleft, proprietary

	// Remediation
	Remediation string   `json:"remediation" db:"remediation"`
	References  []string `json:"references" db:"references"` // URLs for more info

	// Metadata
	RawOutput string    `json:"raw_output" db:"raw_output"` // Original scanner output (JSON)
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// GetSeverityPriority returns a numeric priority for sorting (higher = more severe)
func (s Severity) GetPriority() int {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}