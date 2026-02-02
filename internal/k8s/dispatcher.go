package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	log "github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// JobDispatcher implements interfaces.JobDispatcher using Kubernetes
type JobDispatcher struct {
	clientset     *kubernetes.Clientset
	config        *interfaces.JobConfig
	storageClient interfaces.StorageClient
	logger        *log.Entry
}

// NewJobDispatcher creates a new Kubernetes job dispatcher
func NewJobDispatcher(clientset *kubernetes.Clientset, config *interfaces.JobConfig, storageClient interfaces.StorageClient) interfaces.JobDispatcher {
	return &JobDispatcher{
		clientset:     clientset,
		config:        config,
		storageClient: storageClient,
		logger:        log.WithField("component", "k8s-dispatcher"),
	}
}

// CreateJob creates a new Kubernetes Job for a scan
func (d *JobDispatcher) CreateJob(ctx context.Context, scan *domain.Scan) (*batchv1.Job, error) {
	logger := d.logger.WithField("scan_id", scan.ID.String())
	logger.Info("Creating Kubernetes job for scan")

	// If this is an artifact-based scan, get the presigned download URL
	var downloadURL string
	if scan.SourceArchiveKey != "" {
		logger.WithField("artifact_id", scan.SourceArchiveKey).Info("Fetching artifact download URL")

		artifactResp, err := d.storageClient.GetArtifact(ctx, scan.SourceArchiveKey)
		if err != nil {
			logger.WithError(err).Error("Failed to get artifact download URL")
			return nil, fmt.Errorf("failed to get artifact download URL: %w", err)
		}

		downloadURL = artifactResp.SignedURL
		logger.WithField("expiration", artifactResp.Expiration).Info("Retrieved artifact download URL")
	}

	// Generate job name (must be DNS-1123 compliant)
	jobName := fmt.Sprintf("scan-%s", scan.ID.String()[:8])

	// Build job spec with download URL
	job := d.buildJobSpec(jobName, scan, downloadURL)

	// Create job in Kubernetes
	createdJob, err := d.clientset.BatchV1().Jobs(d.config.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		logger.WithError(err).Error("Failed to create Kubernetes job")
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	logger.WithField("job_name", jobName).Info("Successfully created Kubernetes job")
	return createdJob, nil
}

// GetJob retrieves a Kubernetes Job by name
func (d *JobDispatcher) GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	job, err := d.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("job not found: %s", name)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return job, nil
}

// DeleteJob deletes a Kubernetes Job
func (d *JobDispatcher) DeleteJob(ctx context.Context, namespace, name string) error {
	logger := d.logger.WithFields(log.Fields{
		"namespace": namespace,
		"job_name":  name,
	})

	deletePolicy := metav1.DeletePropagationBackground
	err := d.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Warn("Job not found, already deleted")
			return nil
		}
		logger.WithError(err).Error("Failed to delete job")
		return fmt.Errorf("failed to delete job: %w", err)
	}

	logger.Info("Successfully deleted Kubernetes job")
	return nil
}

// ListJobs lists all jobs in a namespace with optional label selector
func (d *JobDispatcher) ListJobs(ctx context.Context, namespace string, labelSelector string) (*batchv1.JobList, error) {
	jobs, err := d.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	return jobs, nil
}

// GetJobStatus returns the current status of a job
func (d *JobDispatcher) GetJobStatus(ctx context.Context, namespace, name string) (*interfaces.JobStatus, error) {
	job, err := d.GetJob(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	status := &interfaces.JobStatus{
		Name:      job.Name,
		Namespace: job.Namespace,
		Active:    job.Status.Active,
		Succeeded: job.Status.Succeeded,
		Failed:    job.Status.Failed,
	}

	if job.Status.StartTime != nil {
		startTime := job.Status.StartTime.Format(time.RFC3339)
		status.StartTime = &startTime
	}

	if job.Status.CompletionTime != nil {
		completionTime := job.Status.CompletionTime.Format(time.RFC3339)
		status.CompletionTime = &completionTime
	}

	// Convert conditions
	for _, cond := range job.Status.Conditions {
		status.Conditions = append(status.Conditions, interfaces.JobCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// Get pod name for logs
	podName, err := d.getPodNameForJob(ctx, namespace, name)
	if err == nil {
		status.PodName = podName
	}

	return status, nil
}

// GetJobLogs retrieves logs from the job's pod
func (d *JobDispatcher) GetJobLogs(ctx context.Context, namespace, name string) (string, error) {
	podName, err := d.getPodNameForJob(ctx, namespace, name)
	if err != nil {
		return "", err
	}

	req := d.clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{})
	logs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to stream logs: %w", err)
	}
	defer logs.Close()

	buf := make([]byte, 2000)
	n, err := logs.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(buf[:n]), nil
}

// WatchJob watches for job status changes
func (d *JobDispatcher) WatchJob(ctx context.Context, namespace, name string) (<-chan *interfaces.JobStatus, error) {
	watcher, err := d.clientset.BatchV1().Jobs(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	statusChan := make(chan *interfaces.JobStatus)

	go func() {
		defer close(statusChan)
		defer watcher.Stop()

		for {
			select {
			case event, ok := <-watcher.ResultChan():
				if !ok {
					d.logger.Debug("Watcher channel closed")
					return
				}

				if event.Type == watch.Error {
					d.logger.Error("Watch error event received")
					return
				}

				job, ok := event.Object.(*batchv1.Job)
				if !ok {
					d.logger.Error("Failed to cast event object to Job")
					continue
				}

				status := &interfaces.JobStatus{
					Name:      job.Name,
					Namespace: job.Namespace,
					Active:    job.Status.Active,
					Succeeded: job.Status.Succeeded,
					Failed:    job.Status.Failed,
				}

				if job.Status.StartTime != nil {
					startTime := job.Status.StartTime.Format(time.RFC3339)
					status.StartTime = &startTime
				}

				if job.Status.CompletionTime != nil {
					completionTime := job.Status.CompletionTime.Format(time.RFC3339)
					status.CompletionTime = &completionTime
				}

				select {
				case statusChan <- status:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				d.logger.Debug("Context cancelled, stopping watcher")
				return
			}
		}
	}()

	return statusChan, nil
}

// CancelJob cancels a running job
func (d *JobDispatcher) CancelJob(ctx context.Context, namespace, name string) error {
	logger := d.logger.WithFields(log.Fields{
		"namespace": namespace,
		"job_name":  name,
	})

	// Get the job
	job, err := d.GetJob(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Delete pods associated with the job
	deletePolicy := metav1.DeletePropagationForeground
	err = d.clientset.CoreV1().Pods(namespace).DeleteCollection(ctx, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", job.Name),
	})

	if err != nil {
		logger.WithError(err).Error("Failed to delete job pods")
		return fmt.Errorf("failed to delete job pods: %w", err)
	}

	logger.Info("Successfully cancelled job by deleting pods")
	return nil
}

// buildJobSpec constructs a Kubernetes Job specification for a scan
func (d *JobDispatcher) buildJobSpec(jobName string, scan *domain.Scan, downloadURL string) *batchv1.Job {
	// Convert scan types to comma-separated string
	scanTypes := make([]string, len(scan.ScanTypes))
	for i, st := range scan.ScanTypes {
		scanTypes[i] = string(st)
	}
	scanTypesStr := ""
	for i, st := range scanTypes {
		if i > 0 {
			scanTypesStr += ","
		}
		scanTypesStr += st
	}

	// Build environment variables for the runner
	env := []corev1.EnvVar{
		{Name: "SCAN_ID", Value: scan.ID.String()},
		{Name: "ORGANIZATION_ID", Value: scan.OrganizationID.String()},
		{Name: "PROJECT_ID", Value: scan.ProjectID.String()},
		{Name: "SCAN_TYPES", Value: scanTypesStr},
		{Name: "SOURCE_ARTIFACT_ID", Value: scan.SourceArchiveKey},
		{Name: "REPOSITORY_URL", Value: scan.RepositoryURL},
		{Name: "BRANCH", Value: scan.Branch},
		{Name: "ORCHESTRATOR_ENDPOINT", Value: d.config.OrchestratorEndpoint},
		{Name: "STORAGE_SERVICE_ENDPOINT", Value: d.config.StorageServiceEndpoint},
	}

	// Add download URL if this is an artifact-based scan
	if downloadURL != "" {
		env = append(env, corev1.EnvVar{Name: "SOURCE_DOWNLOAD_URL", Value: downloadURL})
	}

	if scan.CommitSHA != "" {
		env = append(env, corev1.EnvVar{Name: "COMMIT_SHA", Value: scan.CommitSHA})
	}

	// Build resource requirements
	resources := corev1.ResourceRequirements{}
	if d.config.Resources.Requests.CPU != "" || d.config.Resources.Requests.Memory != "" {
		resources.Requests = corev1.ResourceList{}
		if d.config.Resources.Requests.CPU != "" {
			resources.Requests[corev1.ResourceCPU] = parseQuantity(d.config.Resources.Requests.CPU)
		}
		if d.config.Resources.Requests.Memory != "" {
			resources.Requests[corev1.ResourceMemory] = parseQuantity(d.config.Resources.Requests.Memory)
		}
	}
	if d.config.Resources.Limits.CPU != "" || d.config.Resources.Limits.Memory != "" {
		resources.Limits = corev1.ResourceList{}
		if d.config.Resources.Limits.CPU != "" {
			resources.Limits[corev1.ResourceCPU] = parseQuantity(d.config.Resources.Limits.CPU)
		}
		if d.config.Resources.Limits.Memory != "" {
			resources.Limits[corev1.ResourceMemory] = parseQuantity(d.config.Resources.Limits.Memory)
		}
	}

	// Build container spec
	container := corev1.Container{
		Name:            "runner",
		Image:           fmt.Sprintf("%s:%s", d.config.RunnerImage, d.config.RunnerVersion),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             env,
		Resources:       resources,
	}

	// Build pod spec
	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		Containers:    []corev1.Container{container},
	}

	if d.config.ServiceAccount != "" {
		podSpec.ServiceAccountName = d.config.ServiceAccount
	}

	// Build labels
	labels := map[string]string{
		"app":             "cloudscan-runner",
		"scan-id":         scan.ID.String()[:8],
		"organization-id": scan.OrganizationID.String(),
		"project-id":      scan.ProjectID.String(),
	}

	// Build Job spec
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: d.config.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}

	// Set optional job configuration
	if d.config.TTLSecondsAfterFinished != nil {
		job.Spec.TTLSecondsAfterFinished = d.config.TTLSecondsAfterFinished
	}

	if d.config.BackoffLimit != nil {
		job.Spec.BackoffLimit = d.config.BackoffLimit
	}

	if d.config.ActiveDeadlineSeconds != nil {
		job.Spec.ActiveDeadlineSeconds = d.config.ActiveDeadlineSeconds
	}

	return job
}

// parseQuantity is a helper to parse resource quantity strings
func parseQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		log.WithError(err).WithField("quantity", s).Warn("Failed to parse quantity, using zero")
		return resource.Quantity{}
	}
	return q
}

// getPodNameForJob gets the pod name associated with a job
func (d *JobDispatcher) getPodNameForJob(ctx context.Context, namespace, jobName string) (string, error) {
	pods, err := d.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})

	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for job: %s", jobName)
	}

	return pods.Items[0].Name, nil
}