package runner

import (
	"fmt"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// CheckK8sAvailable checks if Kubernetes is available
func CheckK8sAvailable() (bool, string) {
	kubeconfigPath := ""
	if home := homedir.HomeDir(); home != "" {
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return false, err.Error()
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return false, err.Error()
	}

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return false, err.Error()
	}

	return true, fmt.Sprintf("Kubernetes %s", version.GitVersion)
}
