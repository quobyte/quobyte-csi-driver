package framework

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClientset builds a Kubernetes clientset and REST config from a kubeconfig
// file path (as produced by e2e-tests/test_runner).
func NewClientset(kubeconfigPath string) (*kubernetes.Clientset, *rest.Config, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("building rest config from kubeconfig %s: %w", kubeconfigPath, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("building clientset: %w", err)
	}

	return clientset, restConfig, nil
}
