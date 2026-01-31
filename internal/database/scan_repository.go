package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ScanRepository implements interfaces.ScanRepository using PostgreSQL
type ScanRepository struct {
	db *DB
}

// NewScanRepository creates a new ScanRepository
func NewScanRepository(db *DB) interfaces.ScanRepository {
	return &ScanRepository{db: db}
}

// Create creates a new scan
func (r *ScanRepository) Create(ctx context.Context, scan *domain.Scan) error {
	query := `
		INSERT INTO scans (
			id, organization_id, project_id, user_id, status, scan_types,
			repository_url, branch, commit_sha, source_archive_key,
			job_name, job_namespace, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		scan.ID,
		scan.OrganizationID,
		scan.ProjectID,
		scan.UserID,
		scan.Status,
		pq.Array(scan.ScanTypes),
		scan.RepositoryURL,
		scan.Branch,
		scan.CommitSHA,
		scan.SourceArchiveKey,
		scan.JobName,
		scan.JobNamespace,
		scan.CreatedAt,
		scan.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create scan: %w", err)
	}

	return nil
}

// Get retrieves a scan by ID
func (r *ScanRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Scan, error) {
	query := `
		SELECT
			id, organization_id, project_id, user_id, status, scan_types,
			repository_url, branch, commit_sha, source_archive_key,
			job_name, job_namespace,
			findings_count, critical_count, high_count, medium_count, low_count,
			started_at, completed_at, error_message,
			created_at, updated_at
		FROM scans
		WHERE id = $1
	`

	scan := &domain.Scan{}
	var scanTypes pq.StringArray

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&scan.ID,
		&scan.OrganizationID,
		&scan.ProjectID,
		&scan.UserID,
		&scan.Status,
		&scanTypes,
		&scan.RepositoryURL,
		&scan.Branch,
		&scan.CommitSHA,
		&scan.SourceArchiveKey,
		&scan.JobName,
		&scan.JobNamespace,
		&scan.FindingsCount,
		&scan.CriticalCount,
		&scan.HighCount,
		&scan.MediumCount,
		&scan.LowCount,
		&scan.StartedAt,
		&scan.CompletedAt,
		&scan.ErrorMessage,
		&scan.CreatedAt,
		&scan.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scan not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scan: %w", err)
	}

	// Convert string array to ScanType array
	scan.ScanTypes = make([]domain.ScanType, len(scanTypes))
	for i, st := range scanTypes {
		scan.ScanTypes[i] = domain.ScanType(st)
	}

	return scan, nil
}

// Update updates an existing scan
func (r *ScanRepository) Update(ctx context.Context, scan *domain.Scan) error {
	query := `
		UPDATE scans SET
			status = $2,
			job_name = $3,
			findings_count = $4,
			critical_count = $5,
			high_count = $6,
			medium_count = $7,
			low_count = $8,
			started_at = $9,
			completed_at = $10,
			error_message = $11,
			updated_at = $12
		WHERE id = $1
	`

	scan.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		scan.ID,
		scan.Status,
		scan.JobName,
		scan.FindingsCount,
		scan.CriticalCount,
		scan.HighCount,
		scan.MediumCount,
		scan.LowCount,
		scan.StartedAt,
		scan.CompletedAt,
		scan.ErrorMessage,
		scan.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update scan: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scan not found")
	}

	return nil
}

// List retrieves scans with optional filters
func (r *ScanRepository) List(ctx context.Context, filter interfaces.ScanFilter) ([]*domain.Scan, error) {
	query := `
		SELECT
			id, organization_id, project_id, user_id, status, scan_types,
			repository_url, branch, commit_sha, source_archive_key,
			job_name, job_namespace,
			findings_count, critical_count, high_count, medium_count, low_count,
			started_at, completed_at, error_message,
			created_at, updated_at
		FROM scans
		WHERE 1=1
	`

	args := []interface{}{}
	argPos := 1

	if filter.OrganizationID != nil {
		query += fmt.Sprintf(" AND organization_id = $%d", argPos)
		args = append(args, *filter.OrganizationID)
		argPos++
	}

	if filter.ProjectID != nil {
		query += fmt.Sprintf(" AND project_id = $%d", argPos)
		args = append(args, *filter.ProjectID)
		argPos++
	}

	if filter.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argPos)
		args = append(args, *filter.UserID)
		argPos++
	}

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *filter.Status)
		argPos++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, filter.Limit)
		argPos++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, filter.Offset)
		argPos++
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list scans: %w", err)
	}
	defer rows.Close()

	scans := []*domain.Scan{}
	for rows.Next() {
		scan := &domain.Scan{}
		var scanTypes pq.StringArray

		err := rows.Scan(
			&scan.ID,
			&scan.OrganizationID,
			&scan.ProjectID,
			&scan.UserID,
			&scan.Status,
			&scanTypes,
			&scan.RepositoryURL,
			&scan.Branch,
			&scan.CommitSHA,
			&scan.SourceArchiveKey,
			&scan.JobName,
			&scan.JobNamespace,
			&scan.FindingsCount,
			&scan.CriticalCount,
			&scan.HighCount,
			&scan.MediumCount,
			&scan.LowCount,
			&scan.StartedAt,
			&scan.CompletedAt,
			&scan.ErrorMessage,
			&scan.CreatedAt,
			&scan.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert string array to ScanType array
		scan.ScanTypes = make([]domain.ScanType, len(scanTypes))
		for i, st := range scanTypes {
			scan.ScanTypes[i] = domain.ScanType(st)
		}

		scans = append(scans, scan)
	}

	return scans, nil
}

// Delete deletes a scan (soft delete - updates status)
func (r *ScanRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE scans SET status = $2, updated_at = $3 WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id, domain.ScanStatusCancelled, time.Now())
	if err != nil {
		return fmt.Errorf("failed to delete scan: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scan not found")
	}

	return nil
}

// UpdateStatus updates only the status of a scan
func (r *ScanRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ScanStatus) error {
	query := `UPDATE scans SET status = $2, updated_at = $3 WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id, status, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update scan status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scan not found")
	}

	return nil
}

// GetByJobName retrieves a scan by Kubernetes job name
func (r *ScanRepository) GetByJobName(ctx context.Context, jobName string) (*domain.Scan, error) {
	query := `
		SELECT
			id, organization_id, project_id, user_id, status, scan_types,
			repository_url, branch, commit_sha, source_archive_key,
			job_name, job_namespace,
			findings_count, critical_count, high_count, medium_count, low_count,
			started_at, completed_at, error_message,
			created_at, updated_at
		FROM scans
		WHERE job_name = $1
	`

	scan := &domain.Scan{}
	var scanTypes pq.StringArray

	err := r.db.QueryRowContext(ctx, query, jobName).Scan(
		&scan.ID,
		&scan.OrganizationID,
		&scan.ProjectID,
		&scan.UserID,
		&scan.Status,
		&scanTypes,
		&scan.RepositoryURL,
		&scan.Branch,
		&scan.CommitSHA,
		&scan.SourceArchiveKey,
		&scan.JobName,
		&scan.JobNamespace,
		&scan.FindingsCount,
		&scan.CriticalCount,
		&scan.HighCount,
		&scan.MediumCount,
		&scan.LowCount,
		&scan.StartedAt,
		&scan.CompletedAt,
		&scan.ErrorMessage,
		&scan.CreatedAt,
		&scan.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scan not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scan by job name: %w", err)
	}

	// Convert string array to ScanType array
	scan.ScanTypes = make([]domain.ScanType, len(scanTypes))
	for i, st := range scanTypes {
		scan.ScanTypes[i] = domain.ScanType(st)
	}

	return scan, nil
}