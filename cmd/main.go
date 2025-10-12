package main

import (
	//"context"
	//"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)

	log.WithFields(log.Fields{
		"version":   version,
		"commit":    commit,
		"buildDate": buildDate,
	}).Info("Starting CloudScan Orchestrator")

	// Get configuration from environment
	grpcPort := getEnv("GRPC_PORT", "9999")
	httpPort := getEnv("HTTP_PORT", "8081")

	// Start HTTP server for health checks
	go startHTTPServer(httpPort)

	// Start gRPC server
	startGRPCServer(grpcPort)
}

func startHTTPServer(port string) {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Health check endpoints
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":    "healthy",
			"service":   "orchestrator",
			"version":   version,
			"commit":    commit,
			"buildDate": buildDate,
			"timestamp": time.Now().UTC(),
		})
	})

	e.GET("/ready", func(c echo.Context) error {
		// TODO: Add readiness checks (DB, Redis, etc.)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": "ready",
		})
	})

	e.GET("/metrics", func(c echo.Context) error {
		// TODO: Prometheus metrics
		return c.String(http.StatusOK, "# Metrics placeholder\n")
	})

	addr := ":" + port
	log.Infof("HTTP server listening on %s", addr)
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

func startGRPCServer(port string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	// TODO: Register gRPC services here
	// pb.RegisterOrchestratorServer(grpcServer, &server{})

	log.Infof("gRPC server listening on :%s", port)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		log.Info("Shutting down gRPC server...")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func init() {
	// Set up logging
	logLevel := getEnv("LOG_LEVEL", "info")
	switch logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	log.Infof("Log level set to: %s", logLevel)
}
