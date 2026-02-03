package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	log "github.com/sirupsen/logrus"
)

// Cleaner enforces data retention policies by cleaning up old scans
type Cleaner struct {
	scanRepo         interfaces.ScanRepository
	findingRepo      interfaces.FindingRepository
	storageClient    interfaces.StorageClient
	jobDispatcher    interfaces.JobDispatcher
	retentionDays    int
	cleanupTime      string // HH:MM format for daily cleanup time
	defaultNamespace string
	logger           *log.Entry
	stopChan         chan struct{}
}

// NewCleaner creates a new cleaner worker
func NewCleaner(
	scanRepo interfaces.ScanRepository,
	findingRepo interfaces.FindingRepository,
	storageClient interfaces.StorageClient,
	jobDispatcher interfaces.JobDispatcher,
	retentionDays int,
	cleanupTime string,
	defaultNamespace string,
) *Cleaner {
	return &Cleaner{
		scanRepo:         scanRepo,
		findingRepo:      findingRepo,
		storageClient:    storageClient,
		jobDispatcher:    jobDispatcher,
		retentionDays:    retentionDays,
		cleanupTime:      cleanupTime,
		defaultNamespace: defaultNamespace,
		logger:           log.WithField("component", "cleaner"),
		stopChan:         make(chan struct{}),
	}
}

// Start begins the cleaner's cleanup schedule
func (c *Cleaner) Start(ctx context.Context) {
	c.logger.WithFields(log.Fields{
		"retention_days": c.retentionDays,
		"cleanup_time":   c.cleanupTime,
	}).Info("Starting cleaner worker")

	// Calculate next cleanup time
	nextCleanup := c.calculateNextCleanup()
	c.logger.WithField("next_cleanup", nextCleanup).Info("Next cleanup scheduled")

	for {
		select {
		case <-time.After(time.Until(nextCleanup)):
			c.cleanup(ctx)
			nextCleanup = c.calculateNextCleanup()
			c.logger.WithField("next_cleanup", nextCleanup).Info("Next cleanup scheduled")

		case <-c.stopChan:
			c.logger.Info("Cleaner worker stopped")
			return

		case <-ctx.Done():
			c.logger.Info("Cleaner worker context cancelled")
			return
		}
	}
}

// Stop gracefully stops the cleaner
func (c *Cleaner) Stop() {
	close(c.stopChan)
}

// calculateNextCleanup determines the next cleanup time based on cleanupTime
func (c *Cleaner) calculateNextCleanup() time.Time {
	now := time.Now()

	// Parse cleanup time (e.g., "00:00" for midnight)
	cleanupHour := 0
	cleanupMinute := 0
	if c.cleanupTime != "" {
		_, _ = fmt.Sscanf(c.cleanupTime, "%d:%d", &cleanupHour, &cleanupMinute)
	}

	// Calculate next cleanup time
	nextCleanup := time.Date(
		now.Year(), now.Month(), now.Day(),
		cleanupHour, cleanupMinute, 0, 0,
		now.Location(),
	)

	// If the time has already passed today, schedule for tomorrow
	if nextCleanup.Before(now) {
		nextCleanup = nextCleanup.Add(24 * time.Hour)
	}

	return nextCleanup
}

// cleanup performs the actual cleanup operation
func (c *Cleaner) cleanup(ctx context.Context) {
	c.logger.Info("Starting cleanup cycle")

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -c.retentionDays)
	c.logger.WithField("cutoff_date", cutoffDate).Info("Cleaning scans older than cutoff date")

	// Query for old scans in FINAL states only (exclude QUEUED and RUNNING)
	// This prevents deleting active scans that are older than retention period
	finalStatuses := []domain.ScanStatus{
		domain.ScanStatusCompleted,
		domain.ScanStatusFailed,
		domain.ScanStatusCancelled,
	}

	var allScansToClean []*domain.Scan

	// Query for each final status
	for _, status := range finalStatuses {
		filter := interfaces.ScanFilter{
			CreatedBefore: &cutoffDate,
			Status:        &status,
		}

		scans, err := c.scanRepo.List(ctx, filter)
		if err != nil {
			c.logger.WithError(err).WithField("status", status).Error("Failed to list old scans")
			continue
		}

		allScansToClean = append(allScansToClean, scans...)
	}

	c.logger.WithField("count", len(allScansToClean)).Info("Found old scans in final states to clean up")

	successCount := 0
	failureCount := 0

	// Clean up each scan
	for _, scan := range allScansToClean {
		if err := c.cleanupScan(ctx, scan); err != nil {
			c.logger.WithError(err).WithField("scan_id", scan.ID.String()).Error("Failed to cleanup scan")
			failureCount++
		} else {
			successCount++
		}
	}

	c.logger.WithFields(log.Fields{
		"success_count": successCount,
		"failure_count": failureCount,
		"total_count":   len(allScansToClean),
	}).Info("Cleanup cycle completed")
}

// cleanupScan cleans up a single scan
func (c *Cleaner) cleanupScan(ctx context.Context, scan *domain.Scan) error {
	logger := c.logger.WithField("scan_id", scan.ID.String())
	logger.Debug("Cleaning up scan")

	// 1. Delete Kubernetes job if it exists
	if scan.JobName != nil && *scan.JobName != "" {
		// Use scan's namespace or fall back to default namespace
		jobNamespace := c.defaultNamespace
		if scan.JobNamespace != nil && *scan.JobNamespace != "" {
			jobNamespace = *scan.JobNamespace
		}
		if err := c.jobDispatcher.DeleteJob(ctx, jobNamespace, *scan.JobName); err != nil {
			logger.WithError(err).Warn("Failed to delete Kubernetes job")
			// Continue cleanup even if job deletion fails
		} else {
			logger.Debug("Deleted Kubernetes job")
		}
	}

	// 2. Collect artifact IDs to delete
	var artifactIDs []string
	if scan.SourceArchiveKey != nil && *scan.SourceArchiveKey != "" {
		artifactIDs = append(artifactIDs, *scan.SourceArchiveKey)
	}
	// TODO: Add results artifact key when implemented

	// 3. Delete artifacts from storage service
	if len(artifactIDs) > 0 {
		if err := c.storageClient.DeleteArtifacts(ctx, artifactIDs); err != nil {
			logger.WithError(err).Warn("Failed to delete artifacts from storage")
			// Continue cleanup even if artifact deletion fails
		} else {
			logger.WithField("artifact_count", len(artifactIDs)).Debug("Deleted artifacts from storage")
		}
	}

	// 4. Delete findings from database
	if err := c.findingRepo.DeleteByScanID(ctx, scan.ID); err != nil {
		logger.WithError(err).Error("Failed to delete findings")
		return err
	}
	logger.Debug("Deleted findings from database")

	// 5. Delete scan from database
	if err := c.scanRepo.Delete(ctx, scan.ID); err != nil {
		logger.WithError(err).Error("Failed to delete scan")
		return err
	}
	logger.Info("Successfully cleaned up scan")

	return nil
}
