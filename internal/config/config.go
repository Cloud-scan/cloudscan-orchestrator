package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the orchestrator service
type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	Redis          RedisConfig
	StorageService StorageServiceConfig
	Kubernetes     KubernetesConfig
	Observability  ObservabilityConfig
}

// ServerConfig holds HTTP/gRPC server configuration
type ServerConfig struct {
	GRPCPort    string
	HTTPPort    string
	MetricsPort string
	Environment string
	LogLevel    string
}

// DatabaseConfig holds PostgreSQL configuration
type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxConnections  int
	MinConnections  int
	MigrationsPath  string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
	TLS      bool
}

// StorageServiceConfig holds storage service gRPC connection configuration
type StorageServiceConfig struct {
	Endpoint string
	Timeout  time.Duration
	TLS      bool
}

// KubernetesConfig holds Kubernetes configuration
type KubernetesConfig struct {
	Namespace               string
	InCluster               bool
	KubeConfigPath          string
	ServiceAccount          string
	RunnerImage             string
	RunnerVersion           string
	TTLSecondsAfterFinished int
	BackoffLimit            int
	ActiveDeadlineSeconds   int
	Resources               ResourceConfig
}

// ResourceConfig holds resource requests and limits
type ResourceConfig struct {
	Requests ResourceList
	Limits   ResourceList
}

// ResourceList holds CPU and memory resources
type ResourceList struct {
	CPU    string
	Memory string
}

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	PrometheusEnabled bool
	JaegerEnabled     bool
	JaegerURL         string
	LogFormat         string // json or text
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort:    getEnv("GRPC_PORT", "9999"),
			HTTPPort:    getEnv("HTTP_PORT", "8081"),
			MetricsPort: getEnv("METRICS_PORT", "9090"),
			Environment: getEnv("ENVIRONMENT", "development"),
			LogLevel:    getEnv("LOG_LEVEL", "info"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "cloudscan"),
			Password:        getEnv("DB_PASSWORD", "changeme"),
			Database:        getEnv("DB_NAME", "orchestrator"),
			SSLMode:         getEnv("DB_SSLMODE", "prefer"),
			MaxConnections:  getEnvInt("DB_MAX_CONNS", 25),
			MinConnections:  getEnvInt("DB_MIN_CONNS", 5),
			MigrationsPath:  getEnv("DB_MIGRATIONS_PATH", "file://migrations"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			TLS:      getEnvBool("REDIS_TLS", false),
		},
		StorageService: StorageServiceConfig{
			Endpoint: getEnv("STORAGE_SERVICE_ENDPOINT", "localhost:8082"),
			Timeout:  getEnvDuration("STORAGE_SERVICE_TIMEOUT", 30*time.Second),
			TLS:      getEnvBool("STORAGE_SERVICE_TLS", false),
		},
		Kubernetes: KubernetesConfig{
			Namespace:               getEnv("KUBE_NAMESPACE", "cloudscan"),
			InCluster:               getEnvBool("KUBE_IN_CLUSTER", false),
			KubeConfigPath:          getEnv("KUBE_CONFIG", ""),
			ServiceAccount:          getEnv("KUBE_SERVICE_ACCOUNT", "cloudscan-runner"),
			RunnerImage:             getEnv("RUNNER_IMAGE", "cloudscan/cloudscan-runner"),
			RunnerVersion:           getEnv("RUNNER_VERSION", "latest"),
			TTLSecondsAfterFinished: getEnvInt("JOB_TTL_SECONDS", 3600),
			BackoffLimit:            getEnvInt("JOB_BACKOFF_LIMIT", 1),
			ActiveDeadlineSeconds:   getEnvInt("JOB_DEADLINE_SECONDS", 3600),
			Resources: ResourceConfig{
				Requests: ResourceList{
					CPU:    getEnv("RUNNER_REQUESTS_CPU", "500m"),
					Memory: getEnv("RUNNER_REQUESTS_MEMORY", "512Mi"),
				},
				Limits: ResourceList{
					CPU:    getEnv("RUNNER_LIMITS_CPU", "2000m"),
					Memory: getEnv("RUNNER_LIMITS_MEMORY", "2Gi"),
				},
			},
		},
		Observability: ObservabilityConfig{
			PrometheusEnabled: getEnvBool("PROMETHEUS_ENABLED", true),
			JaegerEnabled:     getEnvBool("JAEGER_ENABLED", false),
			JaegerURL:         getEnv("JAEGER_URL", ""),
			LogFormat:         getEnv("LOG_FORMAT", "json"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate database config
	if c.Database.Host == "" {
		return fmt.Errorf("DB_HOST is required")
	}
	if c.Database.User == "" {
		return fmt.Errorf("DB_USER is required")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("DB_NAME is required")
	}

	// Validate storage service config
	if c.StorageService.Endpoint == "" {
		return fmt.Errorf("STORAGE_SERVICE_ENDPOINT is required")
	}

	// Validate Kubernetes config
	if c.Kubernetes.Namespace == "" {
		return fmt.Errorf("KUBE_NAMESPACE is required")
	}

	return nil
}

// DSN returns the PostgreSQL connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// GetRedisAddr returns the Redis address
func (c *RedisConfig) GetAddr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

// Helper functions to get environment variables with defaults

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}