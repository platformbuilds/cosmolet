package controller

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GetKubeConfig attempts in-cluster config first, falls back to local config
func GetKubeConfig() (*rest.Config, error) {
	log.Println("Trying in-cluster Kubernetes config...")
	config, err := rest.InClusterConfig()
	if err == nil {
		log.Println("Using in-cluster config")
		return config, nil
	}

	log.Println("Falling back to local kubeconfig")

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not determine home directory: %v", err)
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	log.Println("Using kubeconfig from:", kubeconfig)
	return config, nil
}
