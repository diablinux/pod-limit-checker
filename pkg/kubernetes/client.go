package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Client struct {
	Clientset     *kubernetes.Clientset
	MetricsClient *metricsv.Clientset
}

func NewClient(kubeconfigPath string) (*Client, error) {
	// Build configuration
	config, err := buildConfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Create metrics client
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		// Don't fail if metrics server is not available
		fmt.Println("⚠️  Metrics server not available, continuing without metrics")
		return &Client{
			Clientset: clientset,
		}, nil
	}

	return &Client{
		Clientset:     clientset,
		MetricsClient: metricsClient,
	}, nil
}

func buildConfig(kubeconfigPath string) (*rest.Config, error) {
	// First, try in-cluster config
	if config, err := rest.InClusterConfig(); err == nil {
		fmt.Println("✅ Using in-cluster Kubernetes configuration")
		return config, nil
	}

	// Not in cluster, try kubeconfig
	if kubeconfigPath == "" {
		// Check environment variable
		if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
			kubeconfigPath = envPath
		} else {
			// Try default locations
			home := homeDir()
			possiblePaths := []string{
				filepath.Join(home, ".kube", "config"),
				"/app/kubeconfig",
				"/etc/kubernetes/kubeconfig",
			}

			for _, path := range possiblePaths {
				if _, err := os.Stat(path); err == nil {
					kubeconfigPath = path
					break
				}
			}
		}
	}

	if kubeconfigPath == "" {
		return nil, fmt.Errorf("could not find kubeconfig and not running in-cluster")
	}

	fmt.Printf("✅ Using kubeconfig: %s\n", kubeconfigPath)
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
