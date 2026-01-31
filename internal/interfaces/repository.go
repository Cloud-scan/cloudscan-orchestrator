package interfaces

import (
	"context"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/google/uuid"
)

// ScanRepository defines the interface for scan persistence operations
type ScanRepository interface {
	// Create creates a new scan
	Create(ctx context.Context, scan *domain.Scan) error

	// Get retrieves a scan by ID
	Get(ctx context.Context, id uuid.UUID) (*domain.Scan, error)

	// Update updates an existing scan
	Update(ctx context.Context, scan *domain.Scan) error

	// List retrieves scans with optional filters
	List(ctx context.Context, filter ScanFilter) ([]*domain.Scan, error)

	// Delete deletes a scan (soft delete)
	Delete(ctx context.Context, id uuid.UUID) error

	// UpdateStatus updates only the status of a scan
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ScanStatus) error

	// GetByJobName retrieves a scan by Kubernetes job name
	GetByJobName(ctx context.Context, jobName string) (*domain.Scan, error)
}

// ScanFilter represents filter criteria for listing scans
type ScanFilter struct {
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	UserID         *uuid.UUID
	Status         *domain.ScanStatus
	ScanTypes      []domain.ScanType
	CreatedBefore  *time.Time
	Limit          int
	Offset         int
	PageSize       int
}

// FindingRepository defines the interface for finding persistence operations
type FindingRepository interface {
	// CreateBatch creates multiple findings in a single transaction
	CreateBatch(ctx context.Context, findings []*domain.Finding) error

	// GetByScanID retrieves all findings for a scan
	GetByScanID(ctx context.Context, scanID uuid.UUID) ([]*domain.Finding, error)

	// List retrieves findings with filters
	List(ctx context.Context, filter FindingFilter) ([]*domain.Finding, error)

	// GetStats retrieves finding statistics for a scan
	GetStats(ctx context.Context, scanID uuid.UUID) (*FindingStats, error)

	// DeleteByScanID deletes all findings for a scan
	DeleteByScanID(ctx context.Context, scanID uuid.UUID) error
}

// FindingFilter represents filter criteria for listing findings
type FindingFilter struct {
	ScanID   uuid.UUID
	Severity *domain.Severity
	ScanType *domain.ScanType
	Limit    int
	Offset   int
	PageSize int
}

// FindingStats represents aggregated finding statistics
type FindingStats struct {
	Total      int
	Critical   int
	High       int
	Medium     int
	Low        int
	Info       int
	ByScanType map[domain.ScanType]int
}

// ProjectRepository defines the interface for project persistence operations
type ProjectRepository interface {
	Create(ctx context.Context, project *domain.Project) error
	Get(ctx context.Context, id uuid.UUID) (*domain.Project, error)
	Update(ctx context.Context, project *domain.Project) error
	List(ctx context.Context, organizationID uuid.UUID) ([]*domain.Project, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// OrganizationRepository defines the interface for organization persistence
type OrganizationRepository interface {
	Create(ctx context.Context, org *domain.Organization) error
	Get(ctx context.Context, id uuid.UUID) (*domain.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)
	Update(ctx context.Context, org *domain.Organization) error
	List(ctx context.Context) ([]*domain.Organization, error)
}