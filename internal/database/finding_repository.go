package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// FindingRepository implements interfaces.FindingRepository using PostgreSQL
type FindingRepository struct {
	db     *DB
	logger *log.Entry
}

// NewFindingRepository creates a new finding repository
func NewFindingRepository(db *DB) interfaces.FindingRepository {
	return &FindingRepository{
		db:     db,
		logger: log.WithField("component", "finding-repository"),
	}
}

// CreateBatch creates multiple findings in a single transaction
func (r *FindingRepository) CreateBatch(ctx context.Context, findings []*domain.Finding) error {
	if len(findings) == 0 {
		return nil
	}

	r.logger.WithField("count", len(findings)).Debug("Creating batch of findings")

	// Build bulk insert query - only insert fields that are populated from proto
	query := `INSERT INTO findings (
		id, scan_id, scan_type, tool_name,
		title, description, severity,
		file_path, start_line, code_snippet,
		cwe_id, cve_id, created_at
	) VALUES `

	values := []interface{}{}
	placeholders := []string{}

	for i, f := range findings {
		offset := i * 13
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			offset+1, offset+2, offset+3, offset+4, offset+5, offset+6, offset+7,
			offset+8, offset+9, offset+10, offset+11, offset+12, offset+13,
		))

		values = append(values,
			f.ID,
			f.ScanID,
			f.ScanType,
			f.ToolName,
			f.Title,
			f.Description,
			f.Severity,
			f.FilePath,
			f.StartLine,
			f.CodeSnippet,
			f.CWEID,
			f.CVEID,
			time.Now(),
		)
	}

	query += strings.Join(placeholders, ", ")

	_, err := r.db.ExecContext(ctx, query, values...)
	if err != nil {
		r.logger.WithError(err).Error("Failed to create findings batch")
		return fmt.Errorf("failed to create findings: %w", err)
	}

	r.logger.WithField("count", len(findings)).Info("Successfully created findings batch")
	return nil
}

// GetByScanID retrieves all findings for a scan
func (r *FindingRepository) GetByScanID(ctx context.Context, scanID uuid.UUID) ([]*domain.Finding, error) {
	return r.List(ctx, interfaces.FindingFilter{ScanID: scanID})
}

// List retrieves findings with filters
func (r *FindingRepository) List(ctx context.Context, filter interfaces.FindingFilter) ([]*domain.Finding, error) {
	r.logger.Debug("Listing findings with filters")

	// Only select fields that are actually populated from proto
	query := `SELECT
		id, scan_id, scan_type, tool_name,
		title, description, severity,
		file_path, start_line, code_snippet,
		cwe_id, cve_id, created_at
	FROM findings WHERE 1=1`

	args := []interface{}{}
	argCount := 1

	// Add scan ID filter (required)
	query += fmt.Sprintf(" AND scan_id = $%d", argCount)
	args = append(args, filter.ScanID)
	argCount++

	// Add optional filters
	if filter.ScanType != nil {
		query += fmt.Sprintf(" AND scan_type = $%d", argCount)
		args = append(args, *filter.ScanType)
		argCount++
	}

	if filter.Severity != nil {
		query += fmt.Sprintf(" AND severity = $%d", argCount)
		args = append(args, *filter.Severity)
		argCount++
	}

	// Order by severity (critical -> high -> medium -> low)
	query += " ORDER BY CASE severity WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4 ELSE 5 END, created_at DESC"

	// Add limit
	if filter.PageSize > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filter.PageSize)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.logger.WithError(err).Error("Failed to list findings")
		return nil, fmt.Errorf("failed to list findings: %w", err)
	}
	defer rows.Close()

	findings := []*domain.Finding{}
	for rows.Next() {
		f := &domain.Finding{}
		err := rows.Scan(
			&f.ID, &f.ScanID, &f.ScanType, &f.ToolName,
			&f.Title, &f.Description, &f.Severity,
			&f.FilePath, &f.StartLine, &f.CodeSnippet,
			&f.CWEID, &f.CVEID, &f.CreatedAt,
		)
		if err != nil {
			r.logger.WithError(err).Error("Failed to scan finding row")
			continue
		}
		findings = append(findings, f)
	}

	return findings, nil
}

// GetStats retrieves finding statistics for a scan
func (r *FindingRepository) GetStats(ctx context.Context, scanID uuid.UUID) (*interfaces.FindingStats, error) {
	r.logger.WithField("scan_id", scanID.String()).Debug("Getting finding stats")

	query := `SELECT
		COUNT(*) as total,
		COUNT(CASE WHEN severity = 'critical' THEN 1 END) as critical,
		COUNT(CASE WHEN severity = 'high' THEN 1 END) as high,
		COUNT(CASE WHEN severity = 'medium' THEN 1 END) as medium,
		COUNT(CASE WHEN severity = 'low' THEN 1 END) as low
	FROM findings WHERE scan_id = $1`

	stats := &interfaces.FindingStats{}
	err := r.db.QueryRowContext(ctx, query, scanID).Scan(
		&stats.Total,
		&stats.Critical,
		&stats.High,
		&stats.Medium,
		&stats.Low,
	)
	if err != nil {
		r.logger.WithError(err).Error("Failed to get finding stats")
		return nil, fmt.Errorf("failed to get finding stats: %w", err)
	}

	return stats, nil
}

// DeleteByScanID deletes all findings for a scan
func (r *FindingRepository) DeleteByScanID(ctx context.Context, scanID uuid.UUID) error {
	r.logger.WithField("scan_id", scanID.String()).Debug("Deleting findings by scan ID")

	query := `DELETE FROM findings WHERE scan_id = $1`
	result, err := r.db.ExecContext(ctx, query, scanID)
	if err != nil {
		r.logger.WithError(err).Error("Failed to delete findings")
		return fmt.Errorf("failed to delete findings: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	r.logger.WithField("deleted_count", rowsAffected).Info("Successfully deleted findings")
	return nil
}
