package interfaces

import (
	"context"
	"io"
	"time"
)

// StorageService defines the interface for object storage operations
// Implementation: MinIO (S3-compatible) for local development
// Future: AWS S3, GCS, Azure Blob (unimplemented for now)
type StorageService interface {
	// UploadFile uploads a file to storage
	UploadFile(ctx context.Context, key string, reader io.Reader, contentType string) error

	// DownloadFile downloads a file from storage
	DownloadFile(ctx context.Context, key string) (io.ReadCloser, error)

	// DeleteFile deletes a file from storage
	DeleteFile(ctx context.Context, key string) error

	// GetPresignedUploadURL generates a presigned URL for uploading
	GetPresignedUploadURL(ctx context.Context, key string, expiration time.Duration) (string, error)

	// GetPresignedDownloadURL generates a presigned URL for downloading
	GetPresignedDownloadURL(ctx context.Context, key string, expiration time.Duration) (string, error)

	// FileExists checks if a file exists in storage
	FileExists(ctx context.Context, key string) (bool, error)

	// GetFileMetadata retrieves file metadata
	GetFileMetadata(ctx context.Context, key string) (*FileMetadata, error)

	// ListFiles lists files with a given prefix
	ListFiles(ctx context.Context, prefix string) ([]string, error)
}

// FileMetadata represents metadata about a stored file
type FileMetadata struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
}

// StorageType represents the type of storage backend
type StorageType string

const (
	StorageTypeMinio StorageType = "minio" // MinIO (local development)
	StorageTypeS3    StorageType = "s3"    // AWS S3 (unimplemented)
	StorageTypeGCS   StorageType = "gcs"   // Google Cloud Storage (unimplemented)
	StorageTypeAzure StorageType = "azure" // Azure Blob Storage (unimplemented)
)