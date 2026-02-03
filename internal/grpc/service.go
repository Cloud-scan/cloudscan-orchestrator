package grpc

import (
	"context"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/domain"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	pb "github.com/cloud-scan/cloudscan-orchestrator/generated/proto"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ScanServiceServer implements the gRPC ScanService interface
type ScanServiceServer struct {
	pb.UnimplementedScanServiceServer
	scanRepo      interfaces.ScanRepository
	findingRepo   interfaces.FindingRepository
	storageClient interfaces.StorageClient
	jobDispatcher interfaces.JobDispatcher
	logger        *log.Entry
}

// NewScanServiceServer creates a new gRPC service server
func NewScanServiceServer(
	scanRepo interfaces.ScanRepository,
	findingRepo interfaces.FindingRepository,
	storageClient interfaces.StorageClient,
	jobDispatcher interfaces.JobDispatcher,
) *ScanServiceServer {
	return &ScanServiceServer{
		scanRepo:      scanRepo,
		findingRepo:   findingRepo,
		storageClient: storageClient,
		jobDispatcher: jobDispatcher,
		logger:        log.WithField("component", "grpc-service"),
	}
}

// CreateScan creates a new security scan
func (s *ScanServiceServer) CreateScan(ctx context.Context, req *pb.CreateScanRequest) (*pb.CreateScanResponse, error) {
	logger := s.logger.WithFields(log.Fields{
		"org_id":     req.OrganizationId,
		"project_id": req.ProjectId,
		"git_url":    req.GitUrl,
	})
	logger.Info("Creating new scan")

	// Validate request
	if req.OrganizationId == "" {
		return nil, status.Error(codes.InvalidArgument, "organization_id is required")
	}
	if req.ProjectId == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id is required")
	}
	if len(req.ScanTypes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one scan_type is required")
	}

	// Validate source: Either git_url OR source_artifact_id must be provided
	// (API Gateway should fill in git_url from project if neither is provided)
	if req.GitUrl == "" && req.SourceArtifactId == "" {
		return nil, status.Error(codes.InvalidArgument, "either git_url or source_artifact_id is required")
	}

	// Parse UUIDs
	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid organization_id: %v", err)
	}
	projectID, err := uuid.Parse(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid project_id: %v", err)
	}

	// Parse user_id (optional - nullable in DB)
	var userID uuid.UUID
	if req.UserId != "" {
		userID, err = uuid.Parse(req.UserId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid user_id: %v", err)
		}
	}

	// Convert scan types
	scanTypes := make([]domain.ScanType, len(req.ScanTypes))
	for i, st := range req.ScanTypes {
		scanTypes[i] = convertScanTypeFromProto(st)
	}

	// Create scan domain model
	now := time.Now()
	scan := &domain.Scan{
		ID:               uuid.New(),
		OrganizationID:   orgID,
		ProjectID:        projectID,
		UserID:           userID,
		Status:           domain.ScanStatusQueued,
		ScanTypes:        scanTypes,
		RepositoryURL:    stringPtr(req.GitUrl),
		Branch:           stringPtr(req.GitBranch),
		CommitSHA:        stringPtr(req.GitCommit),
		SourceArchiveKey: stringPtr(req.SourceArtifactId),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	// Save scan to database
	if err := s.scanRepo.Create(ctx, scan); err != nil {
		logger.WithError(err).Error("Failed to save scan to database")
		return nil, status.Errorf(codes.Internal, "failed to create scan: %v", err)
	}

	logger.WithField("scan_id", scan.ID.String()).Info("Scan created successfully")

	// Convert to proto and return
	return &pb.CreateScanResponse{
		Scan: convertScanToProto(scan),
	}, nil
}

// GetScan retrieves a scan by ID
func (s *ScanServiceServer) GetScan(ctx context.Context, req *pb.GetScanRequest) (*pb.Scan, error) {
	logger := s.logger.WithField("scan_id", req.Id)
	logger.Debug("Getting scan")

	scanID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid scan_id: %v", err)
	}

	scan, err := s.scanRepo.Get(ctx, scanID)
	if err != nil {
		logger.WithError(err).Error("Failed to get scan")
		return nil, status.Errorf(codes.NotFound, "scan not found: %v", err)
	}

	return convertScanToProto(scan), nil
}

// ListScans lists scans with filtering
func (s *ScanServiceServer) ListScans(ctx context.Context, req *pb.ListScansRequest) (*pb.ListScansResponse, error) {
	logger := s.logger.WithFields(log.Fields{
		"org_id":     req.OrganizationId,
		"project_id": req.ProjectId,
	})
	logger.Debug("Listing scans")

	// Build filter
	filter := interfaces.ScanFilter{
		PageSize: int(req.PageSize),
	}

	if req.OrganizationId != "" {
		orgID, err := uuid.Parse(req.OrganizationId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid organization_id: %v", err)
		}
		filter.OrganizationID = &orgID
	}

	if req.ProjectId != "" {
		projectID, err := uuid.Parse(req.ProjectId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid project_id: %v", err)
		}
		filter.ProjectID = &projectID
	}

	if req.Status != pb.ScanStatus_SCAN_STATUS_UNSPECIFIED {
		status := convertScanStatusFromProto(req.Status)
		filter.Status = &status
	}

	// Query database
	scans, err := s.scanRepo.List(ctx, filter)
	if err != nil {
		logger.WithError(err).Error("Failed to list scans")
		return nil, status.Errorf(codes.Internal, "failed to list scans: %v", err)
	}

	// Convert to proto
	protoScans := make([]*pb.Scan, len(scans))
	for i, scan := range scans {
		protoScans[i] = convertScanToProto(scan)
	}

	return &pb.ListScansResponse{
		Scans:      protoScans,
		TotalCount: int32(len(protoScans)),
	}, nil
}

// CancelScan cancels a running scan
func (s *ScanServiceServer) CancelScan(ctx context.Context, req *pb.CancelScanRequest) (*emptypb.Empty, error) {
	logger := s.logger.WithField("scan_id", req.Id)
	logger.Info("Cancelling scan")

	scanID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid scan_id: %v", err)
	}

	// Get scan
	scan, err := s.scanRepo.Get(ctx, scanID)
	if err != nil {
		logger.WithError(err).Error("Failed to get scan")
		return nil, status.Errorf(codes.NotFound, "scan not found: %v", err)
	}

	// Check if scan can be cancelled
	if scan.Status == domain.ScanStatusCompleted || scan.Status == domain.ScanStatusFailed {
		return nil, status.Error(codes.FailedPrecondition, "scan already completed")
	}
	if scan.Status == domain.ScanStatusCancelled {
		return &emptypb.Empty{}, nil // Already cancelled
	}

	// Cancel the Kubernetes job if running
	if scan.JobName != nil && *scan.JobName != "" {
		jobNamespace := stringValue(scan.JobNamespace)
		jobName := stringValue(scan.JobName)
		if err := s.jobDispatcher.CancelJob(ctx, jobNamespace, jobName); err != nil {
			logger.WithError(err).Warn("Failed to cancel Kubernetes job")
		}
	}

	// Update scan status
	if err := s.scanRepo.UpdateStatus(ctx, scanID, domain.ScanStatusCancelled); err != nil {
		logger.WithError(err).Error("Failed to update scan status")
		return nil, status.Errorf(codes.Internal, "failed to cancel scan: %v", err)
	}

	logger.Info("Scan cancelled successfully")
	return &emptypb.Empty{}, nil
}

// GetFindings retrieves findings for a scan
func (s *ScanServiceServer) GetFindings(ctx context.Context, req *pb.GetFindingsRequest) (*pb.GetFindingsResponse, error) {
	logger := s.logger.WithField("scan_id", req.ScanId)
	logger.Debug("Getting findings")

	scanID, err := uuid.Parse(req.ScanId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid scan_id: %v", err)
	}

	// Build filter
	filter := interfaces.FindingFilter{
		ScanID:   scanID,
		PageSize: int(req.PageSize),
	}

	if req.ScanType != pb.ScanType_SCAN_TYPE_UNSPECIFIED {
		scanType := convertScanTypeFromProto(req.ScanType)
		filter.ScanType = &scanType
	}

	if req.Severity != pb.Severity_SEVERITY_UNSPECIFIED {
		severity := convertSeverityFromProto(req.Severity)
		filter.Severity = &severity
	}

	// Query database
	findings, err := s.findingRepo.List(ctx, filter)
	if err != nil {
		logger.WithError(err).Error("Failed to list findings")
		return nil, status.Errorf(codes.Internal, "failed to get findings: %v", err)
	}

	// Convert to proto
	protoFindings := make([]*pb.Finding, len(findings))
	for i, finding := range findings {
		protoFindings[i] = convertFindingToProto(finding)
	}

	return &pb.GetFindingsResponse{
		Findings:   protoFindings,
		TotalCount: int32(len(protoFindings)),
	}, nil
}

// UpdateScan updates a scan (called by runner jobs)
func (s *ScanServiceServer) UpdateScan(ctx context.Context, req *pb.UpdateScanRequest) (*pb.Scan, error) {
	logger := s.logger.WithField("scan_id", req.Id)
	logger.Info("Updating scan")

	scanID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid scan_id: %v", err)
	}

	// Get existing scan
	scan, err := s.scanRepo.Get(ctx, scanID)
	if err != nil {
		logger.WithError(err).Error("Failed to get scan")
		return nil, status.Errorf(codes.NotFound, "scan not found: %v", err)
	}

	// Update fields
	if req.Status != pb.ScanStatus_SCAN_STATUS_UNSPECIFIED {
		scan.Status = convertScanStatusFromProto(req.Status)
	}

	if req.TotalFindings > 0 {
		scan.FindingsCount = int(req.TotalFindings)
	}

	if req.ErrorMessage != "" {
		scan.ErrorMessage = stringPtr(req.ErrorMessage)
	}

	// Update in database
	if err := s.scanRepo.Update(ctx, scan); err != nil {
		logger.WithError(err).Error("Failed to update scan")
		return nil, status.Errorf(codes.Internal, "failed to update scan: %v", err)
	}

	logger.Info("Scan updated successfully")
	return convertScanToProto(scan), nil
}

// CreateFindings creates findings in batch (called by runner jobs)
func (s *ScanServiceServer) CreateFindings(ctx context.Context, req *pb.CreateFindingsRequest) (*pb.CreateFindingsResponse, error) {
	logger := s.logger.WithFields(log.Fields{
		"scan_id": req.ScanId,
		"count":   len(req.Findings),
	})
	logger.Info("Creating findings")

	if req.ScanId == "" {
		return nil, status.Error(codes.InvalidArgument, "scan_id is required")
	}

	scanID, err := uuid.Parse(req.ScanId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid scan_id: %v", err)
	}

	// Verify scan exists
	_, err = s.scanRepo.Get(ctx, scanID)
	if err != nil {
		logger.WithError(err).Error("Failed to get scan")
		return nil, status.Errorf(codes.NotFound, "scan not found: %v", err)
	}

	// Convert proto findings to domain
	findings := make([]*domain.Finding, len(req.Findings))
	for i, protoFinding := range req.Findings {
		findings[i] = convertFindingFromProto(protoFinding, scanID)
	}

	// Create findings in database
	if err := s.findingRepo.CreateBatch(ctx, findings); err != nil {
		logger.WithError(err).Error("Failed to create findings")
		return nil, status.Errorf(codes.Internal, "failed to create findings: %v", err)
	}

	logger.Info("Findings created successfully")
	return &pb.CreateFindingsResponse{
		CreatedCount: int32(len(findings)),
	}, nil
}

// DeleteScan deletes a scan and all its data (findings, artifacts, k8s job)
func (s *ScanServiceServer) DeleteScan(ctx context.Context, req *pb.DeleteScanRequest) (*emptypb.Empty, error) {
	logger := s.logger.WithField("scan_id", req.Id)
	logger.Info("Deleting scan")

	scanID, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid scan_id: %v", err)
	}

	// Get scan to retrieve job name and artifact keys
	scan, err := s.scanRepo.Get(ctx, scanID)
	if err != nil {
		logger.WithError(err).Error("Failed to get scan")
		return nil, status.Errorf(codes.NotFound, "scan not found: %v", err)
	}

	// 1. Delete Kubernetes job if it exists
	if scan.JobName != nil && *scan.JobName != "" {
		jobNamespace := stringValue(scan.JobNamespace)
		if err := s.jobDispatcher.DeleteJob(ctx, jobNamespace, *scan.JobName); err != nil {
			logger.WithError(err).Warn("Failed to delete Kubernetes job, continuing with cleanup")
		} else {
			logger.Debug("Deleted Kubernetes job")
		}
	}

	// 2. Collect artifact IDs to delete
	var artifactIDs []string
	if scan.SourceArchiveKey != nil && *scan.SourceArchiveKey != "" {
		artifactIDs = append(artifactIDs, *scan.SourceArchiveKey)
	}

	// 3. Delete artifacts from storage service
	if len(artifactIDs) > 0 {
		if err := s.storageClient.DeleteArtifacts(ctx, artifactIDs); err != nil {
			logger.WithError(err).Warn("Failed to delete artifacts from storage, continuing with cleanup")
		} else {
			logger.WithField("artifact_count", len(artifactIDs)).Debug("Deleted artifacts from storage")
		}
	}

	// 4. Delete findings from database
	if err := s.findingRepo.DeleteByScanID(ctx, scan.ID); err != nil {
		logger.WithError(err).Error("Failed to delete findings")
		return nil, status.Errorf(codes.Internal, "failed to delete findings: %v", err)
	}
	logger.Debug("Deleted findings from database")

	// 5. Delete scan from database
	if err := s.scanRepo.Delete(ctx, scan.ID); err != nil {
		logger.WithError(err).Error("Failed to delete scan")
		return nil, status.Errorf(codes.Internal, "failed to delete scan: %v", err)
	}

	logger.Info("Scan deleted successfully")
	return &emptypb.Empty{}, nil
}

// DeleteProjectScans deletes all scans for a project
func (s *ScanServiceServer) DeleteProjectScans(ctx context.Context, req *pb.DeleteProjectScansRequest) (*pb.DeleteProjectScansResponse, error) {
	logger := s.logger.WithField("project_id", req.ProjectId)
	logger.Info("Deleting all scans for project")

	projectID, err := uuid.Parse(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid project_id: %v", err)
	}

	// List all scans for this project
	filter := interfaces.ScanFilter{
		ProjectID: &projectID,
	}
	scans, err := s.scanRepo.List(ctx, filter)
	if err != nil {
		logger.WithError(err).Error("Failed to list project scans")
		return nil, status.Errorf(codes.Internal, "failed to list project scans: %v", err)
	}

	logger.WithField("scan_count", len(scans)).Info("Found scans to delete")

	deletedCount := 0
	for _, scan := range scans {
		// Delete each scan using DeleteScan logic
		if _, err := s.DeleteScan(ctx, &pb.DeleteScanRequest{Id: scan.ID.String()}); err != nil {
			logger.WithError(err).WithField("scan_id", scan.ID.String()).Error("Failed to delete scan")
			// Continue with other scans even if one fails
			continue
		}
		deletedCount++
	}

	logger.WithField("deleted_count", deletedCount).Info("Project scans deleted")
	return &pb.DeleteProjectScansResponse{
		DeletedCount: int32(deletedCount),
	}, nil
}

// Conversion functions

func convertScanToProto(scan *domain.Scan) *pb.Scan {
	protoScan := &pb.Scan{
		Id:             scan.ID.String(),
		OrganizationId: scan.OrganizationID.String(),
		ProjectId:      scan.ProjectID.String(),
		Status:         convertScanStatusToProto(scan.Status),
		GitUrl:         stringValue(scan.RepositoryURL),
		GitBranch:      stringValue(scan.Branch),
		GitCommit:      stringValue(scan.CommitSHA),
		TotalFindings:  int32(scan.FindingsCount),
		CreatedAt:      timestamppb.New(scan.CreatedAt),
		UpdatedAt:      timestamppb.New(scan.UpdatedAt),
	}

	// Convert scan types
	protoScan.ScanTypes = make([]pb.ScanType, len(scan.ScanTypes))
	for i, st := range scan.ScanTypes {
		protoScan.ScanTypes[i] = convertScanTypeToProto(st)
	}

	// Add completion time if available
	if scan.CompletedAt != nil {
		protoScan.CompletedAt = timestamppb.New(*scan.CompletedAt)
	}

	// Add error message if available
	if scan.ErrorMessage != nil && *scan.ErrorMessage != "" {
		protoScan.ErrorMessage = *scan.ErrorMessage
	}

	// Build findings by severity map
	protoScan.FindingsBySeverity = map[string]int32{
		"critical": int32(scan.CriticalCount),
		"high":     int32(scan.HighCount),
		"medium":   int32(scan.MediumCount),
		"low":      int32(scan.LowCount),
	}

	return protoScan
}

func convertFindingToProto(finding *domain.Finding) *pb.Finding {
	return &pb.Finding{
		Id:          finding.ID.String(),
		ScanId:      finding.ScanID.String(),
		ScanType:    convertScanTypeToProto(finding.ScanType),
		Severity:    convertSeverityToProto(finding.Severity),
		Title:       finding.Title,
		Description: finding.Description,
		FilePath:    finding.FilePath,
		LineNumber:  int32(finding.StartLine),
		CveId:       finding.CVEID,
		CweId:       finding.CWEID,
		CreatedAt:   timestamppb.New(finding.CreatedAt),
	}
}

func convertFindingFromProto(protoFinding *pb.Finding, scanID uuid.UUID) *domain.Finding {
	scanType := convertScanTypeFromProto(protoFinding.ScanType)

	// Derive tool name from scan type (required field in database)
	toolName := deriveToolName(scanType)

	return &domain.Finding{
		ID:          uuid.New(),
		ScanID:      scanID,
		ScanType:    scanType,
		ToolName:    toolName,
		Severity:    convertSeverityFromProto(protoFinding.Severity),
		Title:       protoFinding.Title,
		Description: protoFinding.Description,
		FilePath:    protoFinding.FilePath,
		StartLine:   int(protoFinding.LineNumber),
		CodeSnippet: protoFinding.CodeSnippet,
		CVEID:       protoFinding.CveId,
		CWEID:       protoFinding.CweId,
	}
}

// deriveToolName returns the default tool name for a scan type
func deriveToolName(scanType domain.ScanType) string {
	switch scanType {
	case domain.ScanTypeSAST:
		return "semgrep"
	case domain.ScanTypeSCA:
		return "trivy"
	case domain.ScanTypeSecrets:
		return "gitleaks"
	case domain.ScanTypeLicense:
		return "trivy"
	default:
		return "unknown"
	}
}

func convertScanTypeToProto(st domain.ScanType) pb.ScanType {
	switch st {
	case domain.ScanTypeSAST:
		return pb.ScanType_SAST
	case domain.ScanTypeSCA:
		return pb.ScanType_SCA
	case domain.ScanTypeSecrets:
		return pb.ScanType_SECRETS
	case domain.ScanTypeLicense:
		return pb.ScanType_LICENSE
	default:
		return pb.ScanType_SCAN_TYPE_UNSPECIFIED
	}
}

func convertScanTypeFromProto(st pb.ScanType) domain.ScanType {
	switch st {
	case pb.ScanType_SAST:
		return domain.ScanTypeSAST
	case pb.ScanType_SCA:
		return domain.ScanTypeSCA
	case pb.ScanType_SECRETS:
		return domain.ScanTypeSecrets
	case pb.ScanType_LICENSE:
		return domain.ScanTypeLicense
	default:
		return ""
	}
}

func convertScanStatusToProto(status domain.ScanStatus) pb.ScanStatus {
	switch status {
	case domain.ScanStatusQueued:
		return pb.ScanStatus_QUEUED
	case domain.ScanStatusRunning:
		return pb.ScanStatus_RUNNING
	case domain.ScanStatusCompleted:
		return pb.ScanStatus_COMPLETED
	case domain.ScanStatusFailed:
		return pb.ScanStatus_FAILED
	case domain.ScanStatusCancelled:
		return pb.ScanStatus_CANCELLED
	default:
		return pb.ScanStatus_SCAN_STATUS_UNSPECIFIED
	}
}

func convertScanStatusFromProto(status pb.ScanStatus) domain.ScanStatus {
	switch status {
	case pb.ScanStatus_QUEUED:
		return domain.ScanStatusQueued
	case pb.ScanStatus_RUNNING:
		return domain.ScanStatusRunning
	case pb.ScanStatus_COMPLETED:
		return domain.ScanStatusCompleted
	case pb.ScanStatus_FAILED:
		return domain.ScanStatusFailed
	case pb.ScanStatus_CANCELLED:
		return domain.ScanStatusCancelled
	default:
		return ""
	}
}

func convertSeverityToProto(severity domain.Severity) pb.Severity {
	switch severity {
	case domain.SeverityCritical:
		return pb.Severity_CRITICAL
	case domain.SeverityHigh:
		return pb.Severity_HIGH
	case domain.SeverityMedium:
		return pb.Severity_MEDIUM
	case domain.SeverityLow:
		return pb.Severity_LOW
	default:
		return pb.Severity_SEVERITY_UNSPECIFIED
	}
}

func convertSeverityFromProto(severity pb.Severity) domain.Severity {
	switch severity {
	case pb.Severity_CRITICAL:
		return domain.SeverityCritical
	case pb.Severity_HIGH:
		return domain.SeverityHigh
	case pb.Severity_MEDIUM:
		return domain.SeverityMedium
	case pb.Severity_LOW:
		return domain.SeverityLow
	default:
		return ""
	}
}

// stringPtr returns a pointer to the string if it's not empty, otherwise nil
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// stringValue returns the string value from a pointer, or empty string if nil
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}