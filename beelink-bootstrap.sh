#!/bin/bash
# Bootstrap script for Beelink K8s nodes
# Usage: sudo ./beelink-bootstrap.sh <hostname> <ip> [master|worker] [k3s-token]
#
# Examples:
#   sudo ./beelink-bootstrap.sh beelink1 10.0.0.101 master
#   sudo ./beelink-bootstrap.sh beelink2 10.0.0.102 worker <token>
#   sudo ./beelink-bootstrap.sh beelink3 10.0.0.103 worker <token>

set -e

HOSTNAME=${1:-beelink1}
STATIC_IP=${2:-10.0.0.101}
ROLE=${3:-master}
K3S_TOKEN=${4:-}
GATEWAY=10.0.0.1
DNS=10.0.0.1
MAC_IP=10.0.0.50
K3S_MASTER_IP=10.0.0.101

echo "=== Beelink Bootstrap ==="
echo "Hostname: $HOSTNAME"
echo "Static IP: $STATIC_IP"
echo "Role: $ROLE"
echo ""

# 1. Set hostname
echo "[1/6] Setting hostname..."
hostnamectl set-hostname "$HOSTNAME"
echo "127.0.0.1 $HOSTNAME" >> /etc/hosts

# 2. Configure static IP
echo "[2/6] Configuring static IP..."

# Find the primary network interface
IFACE=$(ip route | grep default | awk '{print $5}' | head -1)
echo "Network interface: $IFACE"

# Create netplan config
cat > /etc/netplan/99-static.yaml << EOF
network:
  version: 2
  renderer: networkd
  ethernets:
    $IFACE:
      dhcp4: no
      addresses:
        - $STATIC_IP/24
      routes:
        - to: default
          via: $GATEWAY
      nameservers:
        addresses:
          - $DNS
          - 8.8.8.8
EOF

chmod 600 /etc/netplan/99-static.yaml

# 3. Install essential packages
echo "[3/6] Installing packages..."
apt update
apt install -y nfs-common curl open-iscsi network-manager

# Enable NetworkManager for WiFi
systemctl enable NetworkManager
systemctl start NetworkManager

# 4. Test NFS connectivity to Mac
echo "[4/6] Testing NFS connectivity to Mac ($MAC_IP)..."
if ping -c 1 -W 2 "$MAC_IP" > /dev/null 2>&1; then
    echo "✓ Mac is reachable"
else
    echo "⚠ Warning: Mac ($MAC_IP) not reachable yet (may work after reboot)"
fi

# 5. Install k3s
echo "[5/6] Installing k3s..."
if [ "$ROLE" = "master" ]; then
    echo "Installing k3s server (master node)..."
    curl -sfL https://get.k3s.io | sh -s - server \
        --write-kubeconfig-mode 644 \
        --disable traefik \
        --disable servicelb

    echo ""
    echo "=== K3S TOKEN (save this for worker nodes) ==="
    cat /var/lib/rancher/k3s/server/node-token
    echo ""
else
    echo "Installing k3s agent (worker node)..."
    if [ -z "$K3S_TOKEN" ]; then
        echo "Enter k3s token from master node:"
        read -r K3S_TOKEN
    fi
    curl -sfL https://get.k3s.io | K3S_URL="https://$K3S_MASTER_IP:6443" K3S_TOKEN="$K3S_TOKEN" sh -
fi

# 6. Create tsuite namespace and secrets placeholder
if [ "$ROLE" = "master" ]; then
    echo "[6/6] Setting up tsuite namespace..."
    export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
    kubectl create namespace tsuite --dry-run=client -o yaml | kubectl apply -f -

    echo ""
    echo "To add secrets later, run:"
    echo "  kubectl -n tsuite create secret generic tsuite-secrets \\"
    echo "    --from-literal=OPENAI_API_KEY=sk-... \\"
    echo "    --from-literal=ANTHROPIC_API_KEY=sk-ant-..."
fi

echo ""
echo "=== Bootstrap Complete ==="
echo ""
echo "Next steps:"
echo "1. Apply network config: sudo netplan apply"
echo "2. Reboot: sudo reboot"
echo "3. SSH back in: ssh $USER@$STATIC_IP"
if [ "$ROLE" = "master" ]; then
    echo "4. Verify cluster: kubectl get nodes"
    echo "5. Save the k3s token for worker nodes!"
fi
echo ""
echo "To configure WiFi later:"
echo "  nmcli device wifi list"
echo "  nmcli device wifi connect 'SSID' password 'PASSWORD'"
echo ""
