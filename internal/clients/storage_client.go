package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	storagepb "github.com/cloud-scan/cloudscan-storage/generated/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// StorageGRPCClient implements the StorageClient interface using gRPC
type StorageGRPCClient struct {
	conn   *grpc.ClientConn
	client storagepb.StorageServiceClient
	logger *log.Entry
}

// NewStorageClient creates a new storage service gRPC client
func NewStorageClient(endpoint string, timeout time.Duration, useTLS bool) (interfaces.StorageClient, error) {
	logger := log.WithField("component", "storage-client")
	logger.WithField("endpoint", endpoint).Info("Connecting to storage service")

	// Configure gRPC dial options
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTimeout(timeout),
	}

	// Add TLS credentials if enabled
	if useTLS {
		creds := credentials.NewTLS(nil)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Connect to storage service
	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to storage service: %w", err)
	}

	client := storagepb.NewStorageServiceClient(conn)
	logger.Info("Successfully connected to storage service")

	return &StorageGRPCClient{
		conn:   conn,
		client: client,
		logger: logger,
	}, nil
}

// CreateArtifact creates a new artifact and returns presigned upload URL
func (c *StorageGRPCClient) CreateArtifact(ctx context.Context, req *interfaces.CreateArtifactRequest) (*interfaces.CreateArtifactResponse, error) {
	c.logger.WithFields(log.Fields{
		"filename": req.FileName,
		"type":     req.ArtifactType,
	}).Debug("Creating artifact")

	// Map artifact type to proto enum
	var artifactType storagepb.ArtifactType
	switch req.ArtifactType {
	case interfaces.ArtifactTypeSource:
		artifactType = storagepb.ArtifactType_SOURCE_CODE
	case interfaces.ArtifactTypeResults:
		artifactType = storagepb.ArtifactType_SCAN_RESULTS
	case interfaces.ArtifactTypeLogs:
		artifactType = storagepb.ArtifactType_LOG
	default:
		artifactType = storagepb.ArtifactType_ARTIFACT_TYPE_UNSPECIFIED
	}

	// Map to protobuf request
	pbReq := &storagepb.CreateArtifactRequest{
		ScanId:         "",  // Will be set by caller if needed
		OrganizationId: "",  // Will be set by caller if needed
		Type:           artifactType,
		Filename:       req.FileName,
		ContentType:    "application/zip",
		SizeBytes:      0, // Size not known yet
		ExpiresInHours: 24,
	}

	// Call storage service
	resp, err := c.client.CreateArtifact(ctx, pbReq)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create artifact")
		return nil, fmt.Errorf("failed to create artifact: %w", err)
	}

	return &interfaces.CreateArtifactResponse{
		ArtifactID: resp.Artifact.Id,
		SignedURL:  resp.UploadUrl,
		Expiration: time.Now().Add(24 * time.Hour),
		Protocol:   interfaces.StorageProtocolS3,
	}, nil
}

// GetArtifact retrieves artifact info and returns presigned download URL
func (c *StorageGRPCClient) GetArtifact(ctx context.Context, artifactID string) (*interfaces.GetArtifactResponse, error) {
	c.logger.WithField("artifact_id", artifactID).Debug("Getting artifact")

	// Call storage service
	resp, err := c.client.GetArtifact(ctx, &storagepb.GetArtifactRequest{
		Id:             artifactID,
		ExpiresInHours: 1, // 1 hour default
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to get artifact")
		return nil, fmt.Errorf("failed to get artifact: %w", err)
	}

	return &interfaces.GetArtifactResponse{
		ArtifactID: resp.Artifact.Id,
		SignedURL:  resp.DownloadUrl,
		Expiration: time.Now().Add(1 * time.Hour),
		Protocol:   interfaces.StorageProtocolS3,
	}, nil
}

// DeleteArtifacts marks artifacts for deletion
func (c *StorageGRPCClient) DeleteArtifacts(ctx context.Context, artifactIDs []string) error {
	c.logger.WithField("count", len(artifactIDs)).Debug("Deleting artifacts")

	// Delete each artifact (could be optimized with batch delete in future)
	for _, id := range artifactIDs {
		_, err := c.client.DeleteArtifact(ctx, &storagepb.DeleteArtifactRequest{
			Id: id,
		})
		if err != nil {
			c.logger.WithError(err).WithField("artifact_id", id).Warn("Failed to delete artifact")
			// Continue with other deletions
		}
	}

	return nil
}

// InitiateMultipartUpload starts a multipart upload session
func (c *StorageGRPCClient) InitiateMultipartUpload(ctx context.Context, artifactID string) (string, error) {
	c.logger.WithField("artifact_id", artifactID).Debug("Initiating multipart upload")

	resp, err := c.client.InitiateMultipartUpload(ctx, &storagepb.InitiateMultipartRequest{
		ArtifactId: artifactID,
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to initiate multipart upload")
		return "", fmt.Errorf("failed to initiate multipart upload: %w", err)
	}

	return resp.UploadId, nil
}

// GetMultipartUploadParts gets presigned URLs for multipart upload parts
func (c *StorageGRPCClient) GetMultipartUploadParts(ctx context.Context, artifactID, uploadID string, fromPart, numParts int32) ([]*interfaces.UploadPart, error) {
	c.logger.WithFields(log.Fields{
		"artifact_id": artifactID,
		"upload_id":   uploadID,
		"from_part":   fromPart,
		"num_parts":   numParts,
	}).Debug("Getting multipart upload parts")

	resp, err := c.client.GetMultipartUploadParts(ctx, &storagepb.GetMultipartPartsRequest{
		ArtifactId: artifactID,
		UploadId:   uploadID,
		FromPart:   fromPart,
		NumParts:   numParts,
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to get multipart upload parts")
		return nil, fmt.Errorf("failed to get multipart upload parts: %w", err)
	}

	// Convert to interface type
	parts := make([]*interfaces.UploadPart, len(resp.Parts))
	for i, part := range resp.Parts {
		expiration, _ := time.Parse(time.RFC3339, part.Expiration)
		parts[i] = &interfaces.UploadPart{
			PartNumber: part.PartNumber,
			URL:        part.Url,
			Expiration: expiration,
		}
	}

	return parts, nil
}

// CompleteMultipartUpload completes a multipart upload
func (c *StorageGRPCClient) CompleteMultipartUpload(ctx context.Context, artifactID, uploadID string, parts []*interfaces.CompletedPart) (*interfaces.CompleteMultipartResponse, error) {
	c.logger.WithFields(log.Fields{
		"artifact_id": artifactID,
		"upload_id":   uploadID,
		"parts":       len(parts),
	}).Debug("Completing multipart upload")

	// Convert to protobuf type
	pbParts := make([]*storagepb.CompletedPart, len(parts))
	for i, part := range parts {
		pbParts[i] = &storagepb.CompletedPart{
			PartNumber: part.PartNumber,
			Etag:       part.ETag,
		}
	}

	resp, err := c.client.CompleteMultipartUpload(ctx, &storagepb.CompleteMultipartRequest{
		ArtifactId: artifactID,
		UploadId:   uploadID,
		Parts:      pbParts,
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to complete multipart upload")
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	expiration, _ := time.Parse(time.RFC3339, resp.Expiration)

	return &interfaces.CompleteMultipartResponse{
		URL:        resp.Url,
		Expiration: expiration,
	}, nil
}

// AbortMultipartUpload aborts a multipart upload
func (c *StorageGRPCClient) AbortMultipartUpload(ctx context.Context, artifactID, uploadID string) error {
	c.logger.WithFields(log.Fields{
		"artifact_id": artifactID,
		"upload_id":   uploadID,
	}).Debug("Aborting multipart upload")

	_, err := c.client.AbortMultipartUpload(ctx, &storagepb.AbortMultipartRequest{
		ArtifactId: artifactID,
		UploadId:   uploadID,
	})
	if err != nil {
		c.logger.WithError(err).Error("Failed to abort multipart upload")
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

// Close closes the gRPC connection
func (c *StorageGRPCClient) Close() error {
	c.logger.Info("Closing storage service connection")
	return c.conn.Close()
}