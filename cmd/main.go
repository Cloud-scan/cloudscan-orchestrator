package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloud-scan/cloudscan-orchestrator/internal/clients"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/config"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/database"
	grpcserver "github.com/cloud-scan/cloudscan-orchestrator/internal/grpc"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/interfaces"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/k8s"
	"github.com/cloud-scan/cloudscan-orchestrator/internal/workers"
	log "github.com/sirupsen/logrus"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// Initialize logger
	initLogger()

	log.WithFields(log.Fields{
		"version":   version,
		"commit":    commit,
		"buildDate": buildDate,
	}).Info("Starting cloudscan-orchestrator")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.WithError(err).Fatal("Failed to load configuration")
	}

	// Initialize database connection
	db, err := database.NewPostgresDB(
		cfg.Database.DSN(),
		cfg.Database.MaxConnections,
		cfg.Database.MinConnections,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer db.Close()

	log.Info("Database connection established")

	// Run migrations
	if err := database.RunMigrations(db.DB, cfg.Database.MigrationsPath); err != nil {
		log.WithError(err).Fatal("Failed to run database migrations")
	}

	// Initialize repositories
	scanRepo := database.NewScanRepository(db)
	findingRepo := database.NewFindingRepository(db)

	// Initialize Kubernetes client
	k8sClient, err := k8s.NewKubernetesClient(
		cfg.Kubernetes.InCluster,
		cfg.Kubernetes.KubeConfigPath,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Kubernetes client")
	}

	// Verify Kubernetes connection
	ctx := context.Background()
	if err := k8s.VerifyConnection(ctx, k8sClient); err != nil {
		log.WithError(err).Fatal("Failed to verify Kubernetes connection")
	}

	// Initialize job dispatcher
	jobConfig := &interfaces.JobConfig{
		Namespace:               cfg.Kubernetes.Namespace,
		ServiceAccount:          cfg.Kubernetes.ServiceAccount,
		RunnerImage:             cfg.Kubernetes.RunnerImage,
		RunnerVersion:           cfg.Kubernetes.RunnerVersion,
		TTLSecondsAfterFinished: int32Ptr(int32(cfg.Kubernetes.TTLSecondsAfterFinished)),
		BackoffLimit:            int32Ptr(int32(cfg.Kubernetes.BackoffLimit)),
		ActiveDeadlineSeconds:   int64Ptr(int64(cfg.Kubernetes.ActiveDeadlineSeconds)),
		OrchestratorEndpoint:    fmt.Sprintf("cloudscan-orchestrator.%s.svc.cluster.local:%s", cfg.Kubernetes.Namespace, cfg.Server.GRPCPort),
		StorageServiceEndpoint:  cfg.StorageService.Endpoint,
		Resources: interfaces.JobResources{
			Requests: interfaces.ResourceList{
				CPU:    cfg.Kubernetes.Resources.Requests.CPU,
				Memory: cfg.Kubernetes.Resources.Requests.Memory,
			},
			Limits: interfaces.ResourceList{
				CPU:    cfg.Kubernetes.Resources.Limits.CPU,
				Memory: cfg.Kubernetes.Resources.Limits.Memory,
			},
		},
	}

	// Initialize storage client (needed by job dispatcher)
	storageClient, err := clients.NewStorageClient(
		cfg.StorageService.Endpoint,
		cfg.StorageService.Timeout,
		cfg.StorageService.TLS,
	)
	if err != nil {
		log.WithError(err).Fatal("Failed to create storage client")
	}
	defer storageClient.Close()

	log.Info("Storage service client initialized")

	// Initialize job dispatcher with storage client
	jobDispatcher := k8s.NewJobDispatcher(k8sClient, jobConfig, storageClient)

	// Initialize gRPC service
	scanService := grpcserver.NewScanServiceServer(
		scanRepo,
		findingRepo,
		storageClient,
		jobDispatcher,
	)

	// Initialize gRPC server
	grpcSrv := grpcserver.NewServer(cfg.Server.GRPCPort, scanService)

	// Initialize HTTP server for health checks and metrics
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.HTTPPort),
		Handler:      setupHTTPHandlers(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Initialize workers
	dispatcher := workers.NewDispatcher(
		scanRepo,
		jobDispatcher,
		10*time.Second, // Check every 10 seconds for queued scans
	)

	sweeper := workers.NewSweeper(
		scanRepo,
		jobDispatcher,
		30*time.Second,          // Check every 30 seconds
		cfg.Kubernetes.Namespace, // Default namespace for jobs
	)

	// Initialize cleaner (may be nil if disabled)
	var cleaner *workers.Cleaner
	if cfg.Workers.EnableCleaner {
		cleaner = workers.NewCleaner(
			scanRepo,
			findingRepo,
			storageClient,
			jobDispatcher,
			90,                       // 90 days retention
			"00:00",                  // Cleanup at midnight
			cfg.Kubernetes.Namespace, // Default namespace for jobs
		)
		log.Info("Cleaner worker enabled")
	} else {
		log.Info("Cleaner worker disabled (set ENABLE_CLEANER=true to enable)")
	}

	// Start background workers
	go dispatcher.Start(ctx)
	go sweeper.Start(ctx)
	if cleaner != nil {
		go cleaner.Start(ctx)
	}

	// Start HTTP server
	go func() {
		log.WithField("port", cfg.Server.HTTPPort).Info("Starting HTTP server")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("HTTP server failed")
		}
	}()

	// Start gRPC server
	go func() {
		if err := grpcSrv.Start(); err != nil {
			log.WithError(err).Fatal("gRPC server failed")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutdown signal received, gracefully stopping...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop workers
	dispatcher.Stop()
	sweeper.Stop()
	if cleaner != nil {
		cleaner.Stop()
	}

	// Stop HTTP server
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("HTTP server shutdown error")
	}

	// Stop gRPC server
	grpcSrv.Stop()

	log.Info("Server stopped")
}

// initLogger configures the logger
func initLogger() {
	log.SetFormatter(&log.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	logLevel, err := log.ParseLevel(level)
	if err != nil {
		logLevel = log.InfoLevel
	}

	log.SetLevel(logLevel)
	log.SetOutput(os.Stdout)
}

// setupHTTPHandlers configures HTTP routes for health and metrics
func setupHTTPHandlers() http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","service":"orchestrator","version":"%s","commit":"%s","buildDate":"%s"}`, version, commit, buildDate)
	})

	// Readiness check endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Add readiness checks (DB connection, K8s API, etc.)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	})

	// Metrics endpoint (Prometheus format)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "# HELP cloudscan_orchestrator_info Build info\n")
		fmt.Fprintf(w, "# TYPE cloudscan_orchestrator_info gauge\n")
		fmt.Fprintf(w, "cloudscan_orchestrator_info{version=\"%s\",commit=\"%s\",buildDate=\"%s\"} 1\n", version, commit, buildDate)
	})

	return mux
}

// Helper functions for pointer conversion
func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}
