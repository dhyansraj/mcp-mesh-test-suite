# Test Suites

> Suite structure and config.yaml

## Directory Structure

```
my-suite/
├── config.yaml           # Suite configuration
├── global/
│   └── routines.yaml     # Global reusable routines (optional)
├── fixtures/             # Files readable via ${fixture:...} (optional)
└── suites/
    ├── uc01_feature_a/
    │   ├── routines.yaml # UC-level routines (optional)
    │   ├── artifacts/    # UC-level shared files (optional)
    │   ├── tc01_test/
    │   │   ├── test.yaml
    │   │   └── artifacts/
    │   └── tc02_test/
    │       └── test.yaml
    └── uc02_feature_b/
        └── tc01_test/
            └── test.yaml
```

## config.yaml

The suite configuration file defines global settings.

```yaml
suite:
  name: My Integration Tests
  mode: docker           # 'docker', 'standalone', or 'k8s'
  disabled: false        # set true to skip the whole suite

docker:
  base_image: python:3.11-slim
  network: bridge

execution:
  max_workers: 4         # Concurrent workers
  timeout: 600           # Overall run timeout in seconds

defaults:
  timeout: 60            # Default test timeout in seconds
  retry: 0               # Default retry count

reports:
  output_dir: reports
  formats:
    - json
  keep_last: 20
```

Recognized top-level keys are `suite`, `packages`, `docker`, `k8s`,
`standalone`, `ssh`, `execution`, `defaults`, `reports`, and `aliases`. Any
other key is ignored by the loader but is still readable through
`${config.<path>}`.

## Execution Modes

### Standalone Mode

Tests run directly on the host machine as subprocesses.

```yaml
suite:
  mode: standalone
```

- Faster execution (no container overhead)
- Tests share host environment
- Good for local development

### Docker Mode

Tests run in isolated containers.

```yaml
suite:
  mode: docker

docker:
  base_image: python:3.11-slim
  network: bridge
```

`docker` accepts exactly two settings: `base_image` (default
`python:3.11-slim`) and `network`, the container network mode (default
`bridge`; may be `host`, `none`, or a user-defined network you created with
`docker network create`). See `tsuite man docker`.

- Full isolation between tests
- Reproducible environment
- Parallel execution support

### K8s Mode

Tests run as pods in a Kubernetes cluster, reading the suite from an NFS
export. Use this to spread a large suite across a cluster.

```yaml
suite:
  mode: k8s

k8s:
  namespace: tsuite                     # default: tsuite
  nfs_server: 10.0.0.50                 # required
  nfs_path: /exports/workspace/my-suite # required, path on the NFS server
  nfs_root: /exports/workspace          # export root, for symlink resolution
  image: my-registry/tsuite-base:latest # overrides docker.base_image
  api_url: http://10.0.0.50:9999        # API URL reachable from pods
  kubeconfig: /home/me/.kube/config     # default: ~/.kube/config
  memory_limit: 4Gi                     # default: 4Gi
  cpu_limit: "2"                        # default: 2
```

| Setting | Description | Required / Default |
|---------|-------------|--------------------|
| `nfs_server` | NFS server address exporting the suite | required |
| `nfs_path` | Path to the suite on the NFS server | required |
| `namespace` | Namespace the worker pods run in | default `tsuite` |
| `nfs_root` | Export root used to resolve symlinks | optional |
| `image` | Worker image; overrides `docker.base_image` | default `python:3.11-slim` |
| `api_url` | API URL pods report to | auto-detected host IP on port 9999 |
| `kubeconfig` | Kubeconfig to use | default `~/.kube/config` |
| `memory_limit` | Pod memory limit | default `4Gi` |
| `cpu_limit` | Pod CPU limit | default `2` |

The run fails immediately with
`k8s.nfs_server and k8s.nfs_path are required in config.yaml` if either is
missing.

### Remote Standalone Mode (SSH)

Standalone mode can also execute on another machine over SSH. The remote host
must be able to read the suite (typically via an NFS mount of the same
directory) and to reach the API server.

```yaml
suite:
  mode: standalone

standalone:
  type: remote                      # 'local' (default) or 'remote'

ssh:
  host: beelink1                    # required for remote; ssh alias or address
  runner_dir: /tmp/tsuite           # where the runner binary is staged
  api_url: http://10.0.0.50:9999    # API URL reachable from the remote host
  local_path: /Users/me/workspace   # local path of the shared export
  mount_path: /mnt/workspace        # where that export is mounted remotely
```

`local_path` and `mount_path` translate the suite path for the remote host: the
`local_path` prefix of the suite path is rewritten to `mount_path`. Verify
connectivity with `tsuite check --suite-path ./my-suite`.

## See Also

- `tsuite man usecases` - Use case organization
- `tsuite man testcases` - Test case structure
- `tsuite man docker` - Docker mode details
