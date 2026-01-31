package grpc

import (
	"fmt"
	"net"

	pb "github.com/cloud-scan/cloudscan-orchestrator/generated/proto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server wraps the gRPC server
type Server struct {
	grpcServer *grpc.Server
	port       string
	logger     *log.Entry
}

// NewServer creates a new gRPC server
func NewServer(port string, scanService *ScanServiceServer) *Server {
	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			loggingInterceptor(),
			errorHandlingInterceptor(),
		),
	)

	// Register services
	pb.RegisterScanServiceServer(grpcServer, scanService)

	// Register reflection service for development
	reflection.Register(grpcServer)

	return &Server{
		grpcServer: grpcServer,
		port:       port,
		logger:     log.WithField("component", "grpc-server"),
	}
}

// Start starts the gRPC server
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", s.port, err)
	}

	s.logger.WithField("port", s.port).Info("Starting gRPC server")

	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve gRPC: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop() {
	s.logger.Info("Stopping gRPC server")
	s.grpcServer.GracefulStop()
}