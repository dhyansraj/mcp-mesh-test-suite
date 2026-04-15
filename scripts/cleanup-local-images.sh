#!/bin/bash
# cleanup-local-images.sh
# Removes old untagged tsuite-mesh images on THIS node
# Checks both k3s/containerd and Docker runtimes
#
# Usage: cleanup-local-images.sh [--dry-run]
# Cron:  0 3 * * * /usr/local/bin/cleanup-local-images.sh >> /tmp/tsuite-image-cleanup.log 2>&1

set -euo pipefail

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
fi

IMAGE_REPO="192.168.10.1:5000/tsuite-mesh"
NODE=$(hostname)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Image cleanup on $NODE"

# --- k3s/containerd cleanup ---
if sudo k3s crictl images >/dev/null 2>&1; then
    local_id=$(sudo k3s crictl images --no-trunc 2>/dev/null | grep "$IMAGE_REPO" | grep ' local ' | awk '{print $3}' | head -1 || true)
    if [[ -n "$local_id" ]]; then
        echo "  [containerd] current: ${local_id:0:18}"
        in_use=$(sudo k3s crictl ps -q 2>/dev/null | xargs -r -I{} sudo k3s crictl inspect {} 2>/dev/null | grep -o '"imageRef":"[^"]*"' | cut -d'"' -f4 | sort -u || true)
        old_images=$(sudo k3s crictl images --no-trunc 2>/dev/null | grep "$IMAGE_REPO" | grep '<none>' | awk '{print $3}' || true)
        count=0
        for img_id in $old_images; do
            [[ "$img_id" == "$local_id" ]] && continue
            if echo "$in_use" | grep -q "$img_id"; then
                echo "  [containerd] SKIP ${img_id:0:18} (in use)"
                continue
            fi
            if $DRY_RUN; then
                echo "  [containerd] WOULD DELETE: ${img_id:0:18}"
                count=$((count + 1))
            else
                sudo k3s crictl rmi "$img_id" 2>/dev/null && count=$((count + 1))
            fi
        done
        echo "  [containerd] Removed: $count"
    fi
fi

# --- Docker cleanup ---
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    local_id=$(docker images "$IMAGE_REPO" --format '{{.ID}} {{.Tag}}' 2>/dev/null | grep ' local' | awk '{print $1}' | head -1 || true)
    if [[ -n "$local_id" ]]; then
        echo "  [docker] current: $local_id"
        old_images=$(docker images "$IMAGE_REPO" --format '{{.ID}} {{.Tag}}' 2>/dev/null | grep '<none>' | awk '{print $1}' || true)
        count=0
        for img_id in $old_images; do
            [[ "$img_id" == "$local_id" ]] && continue
            if $DRY_RUN; then
                echo "  [docker] WOULD DELETE: $img_id"
                count=$((count + 1))
            else
                docker rmi "$img_id" >/dev/null 2>&1 && count=$((count + 1))
            fi
        done
        echo "  [docker] Removed: $count"
    fi
fi
