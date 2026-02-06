package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
)

// K8sExecutor runs tests inside Kubernetes pods
type K8sExecutor struct {
	client    kubernetes.Interface
	namespace string
	image     string
	nfsServer string
	nfsPath   string
	apiURL    string
	suitePath string
	runID     string
}

// NewK8sExecutor creates a new K8s executor
func NewK8sExecutor(cfg *config.SuiteConfig, suitePath, runID string) (*K8sExecutor, error) {
	// Determine kubeconfig path
	kubeconfigPath := cfg.K8s.Kubeconfig
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	// Build config from kubeconfig file
	k8sConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build k8s config: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = clientset.CoreV1().Namespaces().Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to k8s cluster: %w", err)
	}

	// Determine image (k8s.image takes precedence over docker.base_image)
	image := cfg.K8s.Image
	if image == "" {
		image = cfg.Docker.BaseImage
	}
	if image == "" {
		image = "python:3.11-slim"
	}

	// Determine namespace
	namespace := cfg.K8s.Namespace
	if namespace == "" {
		namespace = "tsuite"
	}

	// Validate required NFS settings
	if cfg.K8s.NFSServer == "" {
		return nil, fmt.Errorf("k8s.nfs_server is required")
	}
	if cfg.K8s.NFSPath == "" {
		return nil, fmt.Errorf("k8s.nfs_path is required")
	}
	if cfg.K8s.APIUrl == "" {
		return nil, fmt.Errorf("k8s.api_url is required")
	}

	return &K8sExecutor{
		client:    clientset,
		namespace: namespace,
		image:     image,
		nfsServer: cfg.K8s.NFSServer,
		nfsPath:   cfg.K8s.NFSPath,
		apiURL:    cfg.K8s.APIUrl,
		suitePath: suitePath,
		runID:     runID,
	}, nil
}

// shortID generates a short random ID for pod naming
func shortID() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// sanitizeName converts a test ID to a valid k8s name component
func sanitizeName(s string) string {
	// Replace / with - and lowercase
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ToLower(s)
	// Truncate if too long (k8s names must be <= 63 chars)
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

// ExecuteTest runs a test inside a Kubernetes pod
func (k *K8sExecutor) ExecuteTest(ctx context.Context, testID string, testConfig map[string]any) (*ContainerResult, error) {
	startTime := time.Now()

	// Parse test ID
	parts := strings.Split(testID, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid test ID format: %s", testID)
	}

	// Generate pod name: tsuite-{uc}-{tc}-{shortid}
	podName := fmt.Sprintf("tsuite-%s-%s", sanitizeName(testID), shortID())

	// Calculate NFS subpath for tests (relative path from nfs_path to suite)
	// The suitePath should be under nfsPath
	testsSubPath := ""
	if strings.HasPrefix(k.suitePath, k.nfsPath) {
		testsSubPath = strings.TrimPrefix(k.suitePath, k.nfsPath)
		testsSubPath = strings.TrimPrefix(testsSubPath, "/")
	}

	// Build pod spec
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"app":     "tsuite",
				"run-id":  k.runID,
				"test-id": sanitizeName(testID),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "test",
					Image:   k.image,
					Command: []string{"/usr/local/bin/tsuite-runner"},
					Args: []string{
						"--test-yaml", fmt.Sprintf("/tests/suites/%s/test.yaml", testID),
						"--suite-path", "/tests",
					},
					Env: []corev1.EnvVar{
						{Name: "TSUITE_API", Value: k.apiURL},
						{Name: "TSUITE_RUN_ID", Value: k.runID},
						{Name: "TSUITE_TEST_ID", Value: testID},
					},
					WorkingDir: "/workspace",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "tests",
							MountPath: "/tests",
							SubPath:   testsSubPath,
							ReadOnly:  true,
						},
						{
							Name:      "workspace",
							MountPath: "/workspace",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "tests",
					VolumeSource: corev1.VolumeSource{
						NFS: &corev1.NFSVolumeSource{
							Server:   k.nfsServer,
							Path:     k.nfsPath,
							ReadOnly: true,
						},
					},
				},
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	// Add env from test config
	if testConfig != nil {
		if containerConfigMap, ok := testConfig["container"].(map[string]any); ok {
			if envMap, ok := containerConfigMap["env"].(map[string]any); ok {
				for key, val := range envMap {
					value := fmt.Sprintf("%v", val)
					// Resolve ${env:VAR} references
					if strings.HasPrefix(value, "${env:") && strings.HasSuffix(value, "}") {
						envVar := value[6 : len(value)-1]
						value = os.Getenv(envVar)
					}
					pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{
						Name:  key,
						Value: value,
					})
				}
			}
		}
	}

	// Create pod
	_, err := k.client.CoreV1().Pods(k.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod: %w", err)
	}

	// Cleanup pod after execution
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		k.client.CoreV1().Pods(k.namespace).Delete(cleanupCtx, podName, metav1.DeleteOptions{})
	}()

	// Wait for pod to complete
	result, err := k.waitForPod(ctx, podName)
	if err != nil {
		return &ContainerResult{
			ExitCode: 1,
			Error:    err,
			Duration: time.Since(startTime),
		}, nil
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// waitForPod waits for a pod to complete and returns its result
func (k *K8sExecutor) waitForPod(ctx context.Context, podName string) (*ContainerResult, error) {
	// Poll pod status
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			pod, err := k.client.CoreV1().Pods(k.namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to get pod status: %w", err)
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				logs, _ := k.getPodLogs(ctx, podName)
				return &ContainerResult{
					ExitCode: 0,
					Stdout:   logs,
				}, nil
			case corev1.PodFailed:
				logs, _ := k.getPodLogs(ctx, podName)
				exitCode := 1
				if len(pod.Status.ContainerStatuses) > 0 {
					if term := pod.Status.ContainerStatuses[0].State.Terminated; term != nil {
						exitCode = int(term.ExitCode)
					}
				}
				return &ContainerResult{
					ExitCode: exitCode,
					Stdout:   logs,
					Error:    fmt.Errorf("pod failed with exit code %d", exitCode),
				}, nil
			case corev1.PodPending, corev1.PodRunning:
				// Continue waiting
				continue
			default:
				return nil, fmt.Errorf("unexpected pod phase: %s", pod.Status.Phase)
			}
		}
	}
}

// getPodLogs retrieves logs from a pod
func (k *K8sExecutor) getPodLogs(ctx context.Context, podName string) (string, error) {
	req := k.client.CoreV1().Pods(k.namespace).GetLogs(podName, &corev1.PodLogOptions{})
	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(logs), nil
}

// Close closes the K8s executor (no-op for k8s)
func (k *K8sExecutor) Close() error {
	return nil
}

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
