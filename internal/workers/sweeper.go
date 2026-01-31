package workers

import (
	"context"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	log "github.com/sirupsen/logrus"
)

// Sweeper monitors Kubernetes job status and updates scan records
type Sweeper struct {
	scanRepo      interfaces.ScanRepository
	jobDispatcher interfaces.JobDispatcher
	interval      time.Duration
	logger        *log.Entry
	stopChan      chan struct{}
}

// NewSweeper creates a new sweeper worker
func NewSweeper(
	scanRepo interfaces.ScanRepository,
	jobDispatcher interfaces.JobDispatcher,
	interval time.Duration,
) *Sweeper {
	return &Sweeper{
		scanRepo:      scanRepo,
		jobDispatcher: jobDispatcher,
		interval:      interval,
		logger:        log.WithField("component", "sweeper"),
		stopChan:      make(chan struct{}),
	}
}

// Start begins the sweeper's monitoring loop
func (s *Sweeper) Start(ctx context.Context) {
	s.logger.WithField("interval", s.interval).Info("Starting sweeper worker")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run immediately on start
	s.sweep(ctx)

	for {
		select {
		case <-ticker.C:
			s.sweep(ctx)
		case <-s.stopChan:
			s.logger.Info("Sweeper worker stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Sweeper worker context cancelled")
			return
		}
	}
}

// Stop gracefully stops the sweeper
func (s *Sweeper) Stop() {
	close(s.stopChan)
}

// sweep performs a single sweep operation
func (s *Sweeper) sweep(ctx context.Context) {
	s.logger.Debug("Starting sweep cycle")

	// Query for active scans (queued or running)
	queuedStatus := domain.ScanStatusQueued
	runningStatus := domain.ScanStatusRunning

	queuedScans, err := s.scanRepo.List(ctx, interfaces.ScanFilter{Status: &queuedStatus})
	if err != nil {
		s.logger.WithError(err).Error("Failed to list queued scans")
		return
	}

	runningScans, err := s.scanRepo.List(ctx, interfaces.ScanFilter{Status: &runningStatus})
	if err != nil {
		s.logger.WithError(err).Error("Failed to list running scans")
		return
	}

	allScans := append(queuedScans, runningScans...)
	s.logger.WithField("count", len(allScans)).Debug("Found active scans to check")

	// Check each scan's job status
	for _, scan := range allScans {
		s.processScan(ctx, scan)
	}

	s.logger.Debug("Sweep cycle completed")
}

// processScan checks and updates a single scan
func (s *Sweeper) processScan(ctx context.Context, scan *domain.Scan) {
	logger := s.logger.WithFields(log.Fields{
		"scan_id":  scan.ID.String(),
		"job_name": scan.JobName,
		"status":   scan.Status,
	})

	// If no job name yet, skip (job not created yet)
	if scan.JobName == "" {
		logger.Debug("Scan has no job name, skipping")
		return
	}

	// Get job status from Kubernetes
	jobStatus, err := s.jobDispatcher.GetJobStatus(ctx, scan.JobNamespace, scan.JobName)
	if err != nil {
		logger.WithError(err).Warn("Failed to get job status")
		return
	}

	logger = logger.WithFields(log.Fields{
		"job_active":    jobStatus.Active,
		"job_succeeded": jobStatus.Succeeded,
		"job_failed":    jobStatus.Failed,
	})

	// Update scan based on job status
	var newStatus domain.ScanStatus
	var errorMessage *string

	if jobStatus.Succeeded > 0 {
		// Job completed successfully
		newStatus = domain.ScanStatusCompleted
		logger.Info("Job completed successfully")

	} else if jobStatus.Failed > 0 {
		// Job failed
		newStatus = domain.ScanStatusFailed

		// Try to get error message from job conditions
		for _, cond := range jobStatus.Conditions {
			if cond.Type == "Failed" && cond.Status == "True" {
				msg := cond.Message
				if msg == "" {
					msg = cond.Reason
				}
				errorMessage = &msg
				break
			}
		}

		if errorMessage == nil {
			// Try to get logs
			logs, err := s.jobDispatcher.GetJobLogs(ctx, scan.JobNamespace, scan.JobName)
			if err == nil && logs != "" {
				// Take last 500 chars of logs as error message
				if len(logs) > 500 {
					logs = "..." + logs[len(logs)-500:]
				}
				errorMessage = &logs
			} else {
				defaultMsg := "Job failed with unknown error"
				errorMessage = &defaultMsg
			}
		}

		logger.WithField("error", *errorMessage).Warn("Job failed")

	} else if jobStatus.Active > 0 {
		// Job is running
		if scan.Status != domain.ScanStatusRunning {
			newStatus = domain.ScanStatusRunning
			logger.Info("Job is now running")
		} else {
			// Already running, no update needed
			return
		}
	} else {
		// Job exists but has no active/succeeded/failed pods - might be pending
		logger.Debug("Job has no active/succeeded/failed pods, status unchanged")
		return
	}

	// Update scan status in database
	scan.Status = newStatus
	if errorMessage != nil {
		scan.ErrorMessage = *errorMessage
	}

	if newStatus == domain.ScanStatusCompleted || newStatus == domain.ScanStatusFailed {
		now := time.Now()
		scan.CompletedAt = &now
	}

	if err := s.scanRepo.Update(ctx, scan); err != nil {
		logger.WithError(err).Error("Failed to update scan status")
		return
	}

	logger.WithField("new_status", newStatus).Info("Updated scan status")
}