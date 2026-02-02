package workers

import (
	"context"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	log "github.com/sirupsen/logrus"
)

// Dispatcher picks up queued scans and creates Kubernetes jobs for them
type Dispatcher struct {
	scanRepo      interfaces.ScanRepository
	jobDispatcher interfaces.JobDispatcher
	interval      time.Duration
	logger        *log.Entry
	stopChan      chan struct{}
}

// NewDispatcher creates a new dispatcher worker
func NewDispatcher(
	scanRepo interfaces.ScanRepository,
	jobDispatcher interfaces.JobDispatcher,
	interval time.Duration,
) *Dispatcher {
	return &Dispatcher{
		scanRepo:      scanRepo,
		jobDispatcher: jobDispatcher,
		interval:      interval,
		logger:        log.WithField("component", "dispatcher"),
		stopChan:      make(chan struct{}),
	}
}

// Start begins the dispatcher's processing loop
func (d *Dispatcher) Start(ctx context.Context) {
	d.logger.WithField("interval", d.interval).Info("Starting dispatcher worker")

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Run immediately on start
	d.dispatch(ctx)

	for {
		select {
		case <-ticker.C:
			d.dispatch(ctx)
		case <-d.stopChan:
			d.logger.Info("Dispatcher worker stopped")
			return
		case <-ctx.Done():
			d.logger.Info("Dispatcher worker context cancelled")
			return
		}
	}
}

// Stop gracefully stops the dispatcher
func (d *Dispatcher) Stop() {
	close(d.stopChan)
}

// dispatch processes queued scans
func (d *Dispatcher) dispatch(ctx context.Context) {
	d.logger.Debug("Starting dispatch cycle")

	// Query for queued scans
	queuedStatus := domain.ScanStatusQueued
	scans, err := d.scanRepo.List(ctx, interfaces.ScanFilter{Status: &queuedStatus})
	if err != nil {
		d.logger.WithError(err).Error("Failed to list queued scans")
		return
	}

	if len(scans) == 0 {
		d.logger.Debug("No queued scans to dispatch")
		return
	}

	d.logger.WithField("count", len(scans)).Info("Found queued scans to dispatch")

	// Dispatch each queued scan
	for _, scan := range scans {
		d.dispatchScan(ctx, scan)
	}

	d.logger.Debug("Dispatch cycle completed")
}

// dispatchScan creates a Kubernetes job for a single scan
func (d *Dispatcher) dispatchScan(ctx context.Context, scan *domain.Scan) {
	logger := d.logger.WithFields(log.Fields{
		"scan_id":    scan.ID.String(),
		"project_id": scan.ProjectID.String(),
	})

	logger.Info("Dispatching scan")

	// Create Kubernetes job
	job, err := d.jobDispatcher.CreateJob(ctx, scan)
	if err != nil {
		logger.WithError(err).Error("Failed to create Kubernetes job")

		// Update scan status to failed
		scan.Status = domain.ScanStatusFailed
		errMsg := err.Error()
		scan.ErrorMessage = &errMsg

		if updateErr := d.scanRepo.Update(ctx, scan); updateErr != nil {
			logger.WithError(updateErr).Error("Failed to update scan status to failed")
		}
		return
	}

	// Update scan with job information
	jobName := job.Name
	jobNamespace := job.Namespace
	scan.JobName = &jobName
	scan.JobNamespace = &jobNamespace
	scan.Status = domain.ScanStatusRunning
	now := time.Now()
	scan.StartedAt = &now

	if err := d.scanRepo.Update(ctx, scan); err != nil {
		logger.WithError(err).Error("Failed to update scan with job information")
		return
	}

	logger.WithFields(log.Fields{
		"job_name":      jobName,
		"job_namespace": jobNamespace,
	}).Info("Successfully dispatched scan")
}
