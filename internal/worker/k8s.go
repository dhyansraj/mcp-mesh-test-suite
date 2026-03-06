package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
)

// Ensure K8sHandler satisfies the WorkerHandler interface
var _ WorkerHandler = (*K8sHandler)(nil)

// K8sHandler runs tests inside Kubernetes pods
type K8sHandler struct {
	client    kubernetes.Interface
	namespace string
	image     string
	nfsServer string
	nfsPath   string
	nfsRoot     string
	suitePath   string
	memoryLimit string
	cpuLimit    string
	podStates   sync.Map // podName -> chan *WorkerResult
}

// NewK8sHandler creates a new K8s worker handler
func NewK8sHandler(cfg *config.SuiteConfig, suitePath string) (*K8sHandler, error) {
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

	// Validate required NFS settings
	if cfg.K8s.NFSServer == "" {
		return nil, fmt.Errorf("k8s.nfs_server is required")
	}
	if cfg.K8s.NFSPath == "" {
		return nil, fmt.Errorf("k8s.nfs_path is required")
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

	memLimit := "4Gi"
	if cfg.K8s.MemoryLimit != "" {
		memLimit = cfg.K8s.MemoryLimit
	}
	cpuLim := "2"
	if cfg.K8s.CPULimit != "" {
		cpuLim = cfg.K8s.CPULimit
	}

	return &K8sHandler{
		client:      clientset,
		namespace:   namespace,
		image:       image,
		nfsServer:   cfg.K8s.NFSServer,
		nfsPath:     cfg.K8s.NFSPath,
		nfsRoot:     cfg.K8s.NFSRoot,
		suitePath:   suitePath,
		memoryLimit: memLimit,
		cpuLimit:    cpuLim,
	}, nil
}

func (h *K8sHandler) Name() string { return "k8s" }

func (h *K8sHandler) StartWorker(ctx context.Context, testID string, runID string, apiURL string) (WorkerInfo, error) {
	// Generate pod name
	podName := fmt.Sprintf("tsuite-%s-%s", k8sSanitizeName(testID), k8sShortID())

	// Determine artifact paths
	parts := strings.Split(testID, "/")
	hasUCArtifacts := false
	hasTCArtifacts := false
	if len(parts) >= 1 {
		ucLocalPath := filepath.Join(h.suitePath, "suites", parts[0], "artifacts")
		if info, err := os.Stat(ucLocalPath); err == nil && info.IsDir() {
			hasUCArtifacts = true
		}
	}
	tcLocalPath := filepath.Join(h.suitePath, "suites", testID, "artifacts")
	if info, err := os.Stat(tcLocalPath); err == nil && info.IsDir() {
		hasTCArtifacts = true
	}

	// Compute suite path relative to NFS root (for initContainer mode)
	suiteRelPath := ""
	if h.nfsRoot != "" {
		suiteRelPath = strings.TrimPrefix(h.nfsPath, h.nfsRoot)
		suiteRelPath = strings.TrimPrefix(suiteRelPath, "/")
	}

	// Build pod spec
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: h.namespace,
			Labels: map[string]string{
				"app":     "tsuite",
				"run-id":  runID,
				"test-id": k8sSanitizeName(testID),
			},
		},
		Spec: k8sBuildPodSpec(
			h.image, h.nfsServer, h.nfsPath, h.nfsRoot, suiteRelPath,
			testID, parts, hasUCArtifacts, hasTCArtifacts,
			[]string{
				"--test-yaml", fmt.Sprintf("/tests/suites/%s/test.yaml", testID),
				"--suite-path", "/tests",
			},
			[]corev1.EnvVar{
				{Name: "TSUITE_API", Value: apiURL},
				{Name: "TSUITE_RUN_ID", Value: runID},
				{Name: "TSUITE_TEST_ID", Value: testID},
			},
			corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(h.cpuLimit),
					corev1.ResourceMemory: resource.MustParse(h.memoryLimit),
				},
			},
		),
	}

	// Create pod
	_, err := h.client.CoreV1().Pods(h.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return WorkerInfo{}, fmt.Errorf("failed to create pod: %w", err)
	}

	// Poll briefly for node assignment (best-effort, non-blocking)
	nodeName := ""
	pollCtx, pollCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pollCancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			goto done
		case <-ticker.C:
			p, err := h.client.CoreV1().Pods(h.namespace).Get(pollCtx, podName, metav1.GetOptions{})
			if err != nil {
				goto done
			}
			if p.Spec.NodeName != "" {
				nodeName = p.Spec.NodeName
				goto done
			}
		}
	}
done:

	// Create result channel and store in podStates
	resultCh := make(chan *WorkerResult, 1)
	h.podStates.Store(podName, resultCh)

	return WorkerInfo{
		ID:       podName,
		TestID:   testID,
		PodName:  podName,
		NodeName: nodeName,
	}, nil
}

func (h *K8sHandler) WaitForWorker(ctx context.Context, info WorkerInfo) (*WorkerResult, error) {
	startTime := time.Now()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			pod, err := h.client.CoreV1().Pods(h.namespace).Get(ctx, info.PodName, metav1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to get pod status: %w", err)
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				logs, _ := h.getPodLogs(ctx, info.PodName)
				duration := time.Since(startTime)
				return &WorkerResult{
					Passed:   true,
					Duration: duration,
				}, logNoop(logs)
			case corev1.PodFailed:
				logs, _ := h.getPodLogs(ctx, info.PodName)
				exitCode := 1
				if len(pod.Status.ContainerStatuses) > 0 {
					if term := pod.Status.ContainerStatuses[0].State.Terminated; term != nil {
						exitCode = int(term.ExitCode)
					}
				}
				duration := time.Since(startTime)
				errMsg := fmt.Sprintf("pod failed with exit code %d", exitCode)
				if logs != "" {
					lines := strings.Split(strings.TrimSpace(logs), "\n")
					if len(lines) > 3 {
						lines = lines[len(lines)-3:]
					}
					errMsg = strings.Join(lines, "; ")
				}
				return &WorkerResult{
					Passed:   false,
					Error:    errMsg,
					Duration: duration,
				}, nil
			case corev1.PodPending, corev1.PodRunning:
				continue
			default:
				return nil, fmt.Errorf("unexpected pod phase: %s", pod.Status.Phase)
			}
		}
	}
}

func (h *K8sHandler) CleanupWorker(ctx context.Context, info WorkerInfo) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h.client.CoreV1().Pods(h.namespace).Delete(cleanupCtx, info.PodName, metav1.DeleteOptions{})
	h.podStates.Delete(info.PodName)
	return nil
}

func (h *K8sHandler) Close() error {
	return nil
}

// getPodLogs retrieves logs from a pod
func (h *K8sHandler) getPodLogs(ctx context.Context, podName string) (string, error) {
	req := h.client.CoreV1().Pods(h.namespace).GetLogs(podName, &corev1.PodLogOptions{})
	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(logs), nil
}

// k8sShortID generates a short random ID for pod naming
func k8sShortID() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// k8sSanitizeName converts a test ID to a valid k8s name component
func k8sSanitizeName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ToLower(s)
	if len(s) > 40 {
		s = s[:40]
	}
	s = strings.TrimRight(s, "-.")
	return s
}

// k8sBuildPodSpec builds the complete PodSpec for a test pod.
// When nfsRoot is set, it uses an initContainer to copy artifacts with symlink
// resolution (cp -rL) from the full NFS export into emptyDir volumes.
// When nfsRoot is empty, it falls back to direct NFS artifact volume mounts.
func k8sBuildPodSpec(image, nfsServer, nfsPath, nfsRoot, suiteRelPath string,
	testID string, parts []string, hasUCArtifacts, hasTCArtifacts bool,
	args []string, env []corev1.EnvVar, resources corev1.ResourceRequirements) corev1.PodSpec {

	// Always present volumes
	volumes := []corev1.Volume{
		{Name: "tests", VolumeSource: corev1.VolumeSource{
			NFS: &corev1.NFSVolumeSource{Server: nfsServer, Path: nfsPath, ReadOnly: true},
		}},
		{Name: "workspace", VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}},
	}

	// Always present mounts for main container
	mainMounts := []corev1.VolumeMount{
		{Name: "tests", MountPath: "/tests", ReadOnly: true},
		{Name: "workspace", MountPath: "/workspace"},
	}

	var initContainers []corev1.Container

	if nfsRoot != "" && (hasUCArtifacts || hasTCArtifacts) {
		// InitContainer mode: mount full NFS export, copy with symlink resolution
		volumes = append(volumes, corev1.Volume{
			Name: "nfs-root",
			VolumeSource: corev1.VolumeSource{
				NFS: &corev1.NFSVolumeSource{Server: nfsServer, Path: nfsRoot, ReadOnly: true},
			},
		})

		initMounts := []corev1.VolumeMount{
			{Name: "nfs-root", MountPath: "/nfs", ReadOnly: true},
		}

		initScript := ""

		if hasUCArtifacts {
			volumes = append(volumes, corev1.Volume{
				Name: "uc-artifacts",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})
			mainMounts = append(mainMounts, corev1.VolumeMount{
				Name: "uc-artifacts", MountPath: "/uc-artifacts", ReadOnly: true,
			})
			initMounts = append(initMounts, corev1.VolumeMount{
				Name: "uc-artifacts", MountPath: "/uc-artifacts",
			})
			ucSrc := fmt.Sprintf("/nfs/%s/suites/%s/artifacts/.", suiteRelPath, parts[0])
			initScript += fmt.Sprintf("cp -rL %s /uc-artifacts/ 2>/dev/null || true\n", ucSrc)
			initScript += "find /uc-artifacts -type d \\( -name node_modules -o -name .venv -o -name __pycache__ -o -name target -o -name dist -o -name build \\) -exec rm -rf {} + 2>/dev/null || true\n"
			initScript += "find /uc-artifacts -name package-lock.json -delete 2>/dev/null || true\n"
		}

		if hasTCArtifacts {
			volumes = append(volumes, corev1.Volume{
				Name: "artifacts",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})
			mainMounts = append(mainMounts, corev1.VolumeMount{
				Name: "artifacts", MountPath: "/artifacts", ReadOnly: true,
			})
			initMounts = append(initMounts, corev1.VolumeMount{
				Name: "artifacts", MountPath: "/artifacts",
			})
			tcSrc := fmt.Sprintf("/nfs/%s/suites/%s/artifacts/.", suiteRelPath, testID)
			initScript += fmt.Sprintf("cp -rL %s /artifacts/ 2>/dev/null || true\n", tcSrc)
			initScript += "find /artifacts -type d \\( -name node_modules -o -name .venv -o -name __pycache__ -o -name target -o -name dist -o -name build \\) -exec rm -rf {} + 2>/dev/null || true\n"
			initScript += "find /artifacts -name package-lock.json -delete 2>/dev/null || true\n"
		}

		initContainers = []corev1.Container{{
			Name:            "setup-artifacts",
			Image:           image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"sh", "-c", initScript},
			VolumeMounts:    initMounts,
		}}
	} else if hasUCArtifacts || hasTCArtifacts {
		// Direct NFS mode (no nfsRoot): mount artifact dirs directly
		// Note: symlinks won't resolve in this mode
		if hasUCArtifacts {
			ucNFSPath := nfsPath + "/suites/" + parts[0] + "/artifacts"
			volumes = append(volumes, corev1.Volume{
				Name: "uc-artifacts",
				VolumeSource: corev1.VolumeSource{
					NFS: &corev1.NFSVolumeSource{Server: nfsServer, Path: ucNFSPath, ReadOnly: true},
				},
			})
			mainMounts = append(mainMounts, corev1.VolumeMount{
				Name: "uc-artifacts", MountPath: "/uc-artifacts", ReadOnly: true,
			})
		}
		if hasTCArtifacts {
			tcNFSPath := nfsPath + "/suites/" + testID + "/artifacts"
			volumes = append(volumes, corev1.Volume{
				Name: "artifacts",
				VolumeSource: corev1.VolumeSource{
					NFS: &corev1.NFSVolumeSource{Server: nfsServer, Path: tcNFSPath, ReadOnly: true},
				},
			})
			mainMounts = append(mainMounts, corev1.VolumeMount{
				Name: "artifacts", MountPath: "/artifacts", ReadOnly: true,
			})
		}
	}

	return corev1.PodSpec{
		RestartPolicy:  corev1.RestartPolicyNever,
		InitContainers: initContainers,
		Containers: []corev1.Container{{
			Name:            "test",
			Image:           image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"sh", "-c", k8sRunnerScript(args)},
			Env:             env,
			WorkingDir:      "/workspace",
			Resources:       resources,
			VolumeMounts:    mainMounts,
		}},
		Volumes: volumes,
	}
}

// k8sRunnerScript generates a shell script that downloads the runner from the API and executes it.
// This ensures k8s pods always use the latest runner without needing to rebuild the Docker image.
// The TSUITE_API env var is already set on the pod.
func k8sRunnerScript(args []string) string {
	// Quote args for shell safety
	quotedArgs := ""
	for _, arg := range args {
		quotedArgs += fmt.Sprintf(" %q", arg)
	}

	return fmt.Sprintf(`set -e
# Detect architecture and download the correct runner from the API
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  RUNNER=tsuite-runner-linux-amd64 ;;
  aarch64) RUNNER=tsuite-runner-linux-arm64 ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Download runner from API (TSUITE_API is set as env var)
echo "Fetching runner from $TSUITE_API/api/runners/$RUNNER..."
curl -sf -o /tmp/tsuite-runner "$TSUITE_API/api/runners/$RUNNER"
chmod +x /tmp/tsuite-runner

# Execute the runner with test arguments
exec /tmp/tsuite-runner%s`, quotedArgs)
}

// logNoop is a helper that discards the logs string but satisfies the return signature.
// Pod logs are retrieved for diagnostics but the runner reports results via the API.
func logNoop(_ string) error {
	return nil
}
