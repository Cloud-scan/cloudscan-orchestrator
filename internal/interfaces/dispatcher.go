package interfaces

import (
	"context"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	batchv1 "k8s.io/api/batch/v1"
)

// JobDispatcher defines the interface for Kubernetes job operations
type JobDispatcher interface {
	// CreateJob creates a new Kubernetes Job for a scan
	CreateJob(ctx context.Context, scan *domain.Scan) (*batchv1.Job, error)

	// GetJob retrieves a Kubernetes Job by name
	GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error)

	// DeleteJob deletes a Kubernetes Job
	DeleteJob(ctx context.Context, namespace, name string) error

	// ListJobs lists all jobs in a namespace with optional label selector
	ListJobs(ctx context.Context, namespace string, labelSelector string) (*batchv1.JobList, error)

	// GetJobStatus returns the current status of a job
	GetJobStatus(ctx context.Context, namespace, name string) (*JobStatus, error)

	// GetJobLogs retrieves logs from the job's pod
	GetJobLogs(ctx context.Context, namespace, name string) (string, error)

	// WatchJob watches for job status changes
	WatchJob(ctx context.Context, namespace, name string) (<-chan *JobStatus, error)

	// CancelJob cancels a running job
	CancelJob(ctx context.Context, namespace, name string) error
}

// JobStatus represents the status of a Kubernetes Job
type JobStatus struct {
	Name       string
	Namespace  string
	Active     int32
	Succeeded  int32
	Failed     int32
	StartTime  *string
	CompletionTime *string
	Conditions []JobCondition
	PodName    string
}

// JobCondition represents a condition in the job status
type JobCondition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

// ScannerConfig represents configuration for a specific scanner
type ScannerConfig struct {
	Name    string
	Image   string
	Version string
	Enabled bool
	Args    []string
	Env     map[string]string
}

// JobConfig represents configuration for creating Kubernetes Jobs
type JobConfig struct {
	Namespace               string
	ServiceAccount          string
	RunnerImage             string
	RunnerVersion           string
	Scanners                []ScannerConfig
	Resources               JobResources
	TTLSecondsAfterFinished *int32
	BackoffLimit            *int32
	ActiveDeadlineSeconds   *int64
	OrchestratorEndpoint    string // gRPC endpoint for runner to call back
	StorageServiceEndpoint  string // gRPC endpoint for storage service
}

// JobResources represents resource requests and limits for a job
type JobResources struct {
	Requests ResourceList
	Limits   ResourceList
}

// ResourceList represents CPU and memory resources
type ResourceList struct {
	CPU    string
	Memory string
}