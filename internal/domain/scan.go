package domain

import (
	"time"

	"github.com/google/uuid"
)

// ScanStatus represents the current state of a scan
type ScanStatus string

const (
	ScanStatusQueued    ScanStatus = "queued"
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusCancelled ScanStatus = "cancelled"
)

// ScanType represents the type of security scan
type ScanType string

const (
	ScanTypeSAST    ScanType = "sast"    // Static Application Security Testing (Semgrep)
	ScanTypeSCA     ScanType = "sca"     // Software Composition Analysis (Trivy)
	ScanTypeSecrets ScanType = "secrets" // Secret Scanning (TruffleHog)
	ScanTypeLicense ScanType = "license" // License Compliance (ScanCode)
)

// Scan represents a security scan request and its state
type Scan struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	OrganizationID uuid.UUID  `json:"organization_id" db:"organization_id"`
	ProjectID      uuid.UUID  `json:"project_id" db:"project_id"`
	UserID         uuid.UUID  `json:"user_id" db:"user_id"`
	Status         ScanStatus `json:"status" db:"status"`
	ScanTypes      []ScanType `json:"scan_types" db:"scan_types"`

	// Source code information
	RepositoryURL string `json:"repository_url" db:"repository_url"`
	Branch        string `json:"branch" db:"branch"`
	CommitSHA     string `json:"commit_sha" db:"commit_sha"`

	// Artifact storage
	SourceArchiveKey string `json:"source_archive_key" db:"source_archive_key"` // S3/MinIO key

	// Kubernetes job information
	JobName      string `json:"job_name" db:"job_name"`
	JobNamespace string `json:"job_namespace" db:"job_namespace"`

	// Results
	FindingsCount  int       `json:"findings_count" db:"findings_count"`
	CriticalCount  int       `json:"critical_count" db:"critical_count"`
	HighCount      int       `json:"high_count" db:"high_count"`
	MediumCount    int       `json:"medium_count" db:"medium_count"`
	LowCount       int       `json:"low_count" db:"low_count"`
	StartedAt      *time.Time `json:"started_at" db:"started_at"`
	CompletedAt    *time.Time `json:"completed_at" db:"completed_at"`
	ErrorMessage   string     `json:"error_message" db:"error_message"`

	// Audit
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsTerminal returns true if the scan is in a terminal state
func (s *Scan) IsTerminal() bool {
	return s.Status == ScanStatusCompleted ||
		s.Status == ScanStatusFailed ||
		s.Status == ScanStatusCancelled
}

// Duration returns the duration of the scan
func (s *Scan) Duration() time.Duration {
	if s.StartedAt == nil {
		return 0
	}

	endTime := time.Now()
	if s.CompletedAt != nil {
		endTime = *s.CompletedAt
	}

	return endTime.Sub(*s.StartedAt)
}

// Project represents a code project to be scanned
type Project struct {
	ID             uuid.UUID `json:"id" db:"id"`
	OrganizationID uuid.UUID `json:"organization_id" db:"organization_id"`
	Name           string    `json:"name" db:"name"`
	Slug           string    `json:"slug" db:"slug"`
	Description    string    `json:"description" db:"description"`
	RepositoryURL  string    `json:"repository_url" db:"repository_url"`
	DefaultBranch  string    `json:"default_branch" db:"default_branch"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// Organization represents a tenant in the multi-tenant system
type Organization struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Slug      string    `json:"slug" db:"slug"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}