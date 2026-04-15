#!/bin/bash
# cleanup-node-images.sh
# Removes old untagged tsuite-mesh images from k3s/Docker nodes
# Keeps: current :local tagged image, images in use by running containers
#
# Usage: ./cleanup-node-images.sh [--dry-run]

set -euo pipefail

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    echo "=== DRY RUN MODE ==="
fi

NODES="beelink1 beelink2 beelink3 beelink4"
IMAGE_REPO="192.168.10.1:5000/tsuite-mesh"

for node in $NODES; do
    echo ""
    echo "=== $node ==="

    # Detect runtime: try k3s crictl first, fall back to docker
    has_crictl=$(ssh "$node" "sudo k3s crictl images 2>/dev/null | head -1" 2>/dev/null || echo "")
    has_docker=$(ssh "$node" "docker images --format '{{.Repository}}' 2>/dev/null | head -1" 2>/dev/null || echo "")

    if [[ -n "$has_crictl" ]]; then
        RUNTIME="crictl"
        # Get current :local image ID
        local_id=$(ssh "$node" "sudo k3s crictl images --no-trunc 2>/dev/null | grep '$IMAGE_REPO' | grep ' local ' | awk '{print \$3}'" 2>/dev/null | head -1)

        if [[ -z "$local_id" ]]; then
            echo "  No tsuite-mesh:local image found, skipping"
            continue
        fi
        echo "  Runtime: k3s/containerd"
        echo "  Current :local image: ${local_id:0:18}"

        # Get IDs of images in use by running containers
        in_use=$(ssh "$node" "sudo k3s crictl ps -q 2>/dev/null | xargs -r -I{} sudo k3s crictl inspect {} 2>/dev/null | grep -o '\"imageRef\":\"[^\"]*\"' | cut -d'\"' -f4 | sort -u" 2>/dev/null || echo "")

        # Get all old untagged tsuite-mesh images
        old_images=$(ssh "$node" "sudo k3s crictl images --no-trunc 2>/dev/null | grep '$IMAGE_REPO' | grep '<none>' | awk '{print \$3}'" 2>/dev/null || echo "")

        if [[ -z "$old_images" ]]; then
            echo "  No old images to clean"
            continue
        fi

        count=0
        skipped=0
        for img_id in $old_images; do
            if [[ "$img_id" == "$local_id" ]]; then
                continue
            fi
            if echo "$in_use" | grep -q "$img_id"; then
                echo "  SKIP ${img_id:0:18} (in use)"
                skipped=$((skipped + 1))
                continue
            fi
            if $DRY_RUN; then
                echo "  WOULD DELETE: ${img_id:0:18}"
                count=$((count + 1))
            else
                if ssh "$node" "sudo k3s crictl rmi $img_id 2>/dev/null"; then
                    count=$((count + 1))
                else
                    echo "  FAILED: ${img_id:0:18}"
                fi
            fi
        done
        echo "  Removed: $count, Skipped: $skipped"

    elif [[ -n "$has_docker" ]]; then
        RUNTIME="docker"
        # Get current :local image ID
        local_id=$(ssh "$node" "docker images '$IMAGE_REPO' --format '{{.ID}} {{.Tag}}' 2>/dev/null | grep ' local' | awk '{print \$1}'" 2>/dev/null | head -1)

        if [[ -z "$local_id" ]]; then
            echo "  No tsuite-mesh:local image found, skipping"
            continue
        fi
        echo "  Runtime: Docker"
        echo "  Current :local image: $local_id"

        # Get old untagged image IDs
        old_images=$(ssh "$node" "docker images '$IMAGE_REPO' --format '{{.ID}} {{.Tag}}' 2>/dev/null | grep '<none>' | awk '{print \$1}'" 2>/dev/null || echo "")

        if [[ -z "$old_images" ]]; then
            echo "  No old images to clean"
            continue
        fi

        count=0
        for img_id in $old_images; do
            if [[ "$img_id" == "$local_id" ]]; then
                continue
            fi
            if $DRY_RUN; then
                echo "  WOULD DELETE: $img_id"
                count=$((count + 1))
            else
                if ssh "$node" "docker rmi $img_id 2>/dev/null" >/dev/null 2>&1; then
                    count=$((count + 1))
                else
                    echo "  FAILED: $img_id"
                fi
            fi
        done
        echo "  Removed: $count"

    else
        echo "  No container runtime found, skipping"
    fi
done

echo ""
echo "=== Done ==="
