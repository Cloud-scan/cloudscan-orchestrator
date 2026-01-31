package interfaces

import (
	"context"
	"time"
)

// StorageClient defines the interface for communicating with the storage service
// The storage service is a separate microservice (cloudscan-storage) that handles
// object storage operations and provides presigned URLs
type StorageClient interface {
	// CreateArtifact creates a new artifact and returns a presigned upload URL
	CreateArtifact(ctx context.Context, req *CreateArtifactRequest) (*CreateArtifactResponse, error)

	// GetArtifact retrieves artifact info and returns a presigned download URL
	GetArtifact(ctx context.Context, artifactID string) (*GetArtifactResponse, error)

	// DeleteArtifacts marks artifacts for deletion
	DeleteArtifacts(ctx context.Context, artifactIDs []string) error

	// InitiateMultipartUpload starts a multipart upload session
	InitiateMultipartUpload(ctx context.Context, artifactID string) (string, error)

	// GetMultipartUploadParts gets presigned URLs for multipart upload parts
	GetMultipartUploadParts(ctx context.Context, artifactID, uploadID string, fromPart, numParts int32) ([]*UploadPart, error)

	// CompleteMultipartUpload completes a multipart upload
	CompleteMultipartUpload(ctx context.Context, artifactID, uploadID string, parts []*CompletedPart) (*CompleteMultipartResponse, error)

	// AbortMultipartUpload aborts a multipart upload
	AbortMultipartUpload(ctx context.Context, artifactID, uploadID string) error

	// Close closes the gRPC connection
	Close() error
}

// CreateArtifactRequest represents a request to create an artifact
type CreateArtifactRequest struct {
	FileName     string
	FileHash     string
	FileSize     string
	ArtifactType ArtifactType
	UploadType   UploadType
}

// CreateArtifactResponse represents the response from creating an artifact
type CreateArtifactResponse struct {
	ArtifactID string
	SignedURL  string
	Expiration time.Time
	Protocol   StorageProtocol
}

// GetArtifactResponse represents the response from getting an artifact
type GetArtifactResponse struct {
	ArtifactID string
	SignedURL  string
	Expiration time.Time
	Protocol   StorageProtocol
}

// UploadPart represents a presigned URL for uploading a part
type UploadPart struct {
	PartNumber int32
	URL        string
	Expiration time.Time
}

// CompletedPart represents a completed upload part
type CompletedPart struct {
	PartNumber int32
	ETag       string
}

// CompleteMultipartResponse represents the response from completing multipart upload
type CompleteMultipartResponse struct {
	URL        string
	Expiration time.Time
}

// ArtifactType represents the type of artifact
type ArtifactType string

const (
	ArtifactTypeSource  ArtifactType = "source"  // Source code archive
	ArtifactTypeResults ArtifactType = "results" // Scan results
	ArtifactTypeLogs    ArtifactType = "logs"    // Job logs
)

// UploadType represents the upload method
type UploadType string

const (
	UploadTypeSimple    UploadType = "simple"    // Single PUT request
	UploadTypeResumable UploadType = "resumable" // Resumable upload
	UploadTypeMultipart UploadType = "multipart" // Multipart upload
)

// StorageProtocol represents the storage backend protocol
type StorageProtocol string

const (
	StorageProtocolS3        StorageProtocol = "s3"
	StorageProtocolS3Express StorageProtocol = "s3express"
	StorageProtocolAzure     StorageProtocol = "azure"
	StorageProtocolGCS       StorageProtocol = "gcs"
)