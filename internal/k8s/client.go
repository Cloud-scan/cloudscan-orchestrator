package k8s

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewKubernetesClient creates a new Kubernetes client
// It automatically detects in-cluster or out-of-cluster configuration
func NewKubernetesClient(inCluster bool, kubeConfigPath string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if inCluster {
		log.Info("Using in-cluster Kubernetes configuration")
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	} else {
		log.Info("Using out-of-cluster Kubernetes configuration")

		// Use provided kubeconfig path or default to ~/.kube/config
		if kubeConfigPath == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			kubeConfigPath = filepath.Join(homeDir, ".kube", "config")
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	log.Info("Kubernetes client created successfully")
	return clientset, nil
}

// VerifyConnection verifies the Kubernetes client can connect to the cluster
func VerifyConnection(ctx context.Context, clientset *kubernetes.Clientset) error {
	_, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to Kubernetes cluster: %w", err)
	}

	log.Info("Successfully connected to Kubernetes cluster")
	return nil
}