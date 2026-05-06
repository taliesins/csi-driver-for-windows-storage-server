#!/bin/bash

# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

ver="master"
use_local=false
nfs_kerberos=false
nfs_kerberos_flavor="krb5"
node_only=false

usage() {
  cat <<EOF
Usage: $0 [version] [local] [--node-only] [--nfs-kerberos] [--nfs-kerberos-flavor krb5|krb5i|krb5p]

Examples:
  $0
  $0 v1.0.1
  $0 master local --node-only
  $0 master local --nfs-kerberos --nfs-kerberos-flavor krb5p

Before installing controllers, either create Secret/kube-system/csi-driver-winrm
with WINRM_HOST, WINRM_USER, and WINRM_PASSWORD keys, or export those environment
variables and this script will create/update the Secret for you.

Use --node-only for static, pre-provisioned volumes. This installs the Linux
node side and CSIDriver objects only, skips WinRM/controller setup, and disables
CSI attach for iSCSI.
EOF
}

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    local|--local)
      use_local=true
      ;;
    --node-only|--nodeonly)
      node_only=true
      ;;
    --nfs-kerberos|--kerberos)
      nfs_kerberos=true
      ;;
    --nfs-kerberos-flavor|--kerberos-flavor)
      if [[ "$#" -lt 2 ]]; then
        echo "missing value for $1" >&2
        usage >&2
        exit 1
      fi
      nfs_kerberos_flavor="$2"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      ver="$1"
      ;;
  esac
  shift
done

case "$nfs_kerberos_flavor" in
  krb5|krb5i|krb5p)
    ;;
  *)
    echo "invalid --nfs-kerberos-flavor '$nfs_kerberos_flavor'; expected krb5, krb5i, or krb5p" >&2
    exit 1
    ;;
esac

repo="https://raw.githubusercontent.com/taliesins/csi-driver-for-windows-storage-server/$ver/deploy"
if [[ "$use_local" == true ]]; then
  echo "use local deploy"
  repo="./deploy"
fi

chart_path="chart/csi-driver-for-windows-storage-server/Chart.yaml"
driver_image_repository="ghcr.io/taliesins/csi-driver-for-windows-storage-server"

apply_manifest() {
  kubectl apply -f "$repo/$1"
}

chart_app_version() {
  if [[ "$use_local" == true ]]; then
    sed -nE 's/^appVersion:[[:space:]]*"?([^"]+)"?[[:space:]]*$/\1/p' "$chart_path" | head -n 1
  else
    curl -fsSL "https://raw.githubusercontent.com/taliesins/csi-driver-for-windows-storage-server/$ver/$chart_path" \
      | sed -nE 's/^appVersion:[[:space:]]*"?([^"]+)"?[[:space:]]*$/\1/p' \
      | head -n 1
  fi
}

apply_controller_manifest() {
  local app_version
  app_version="$(chart_app_version)"
  if [[ -z "$app_version" ]]; then
    echo "could not determine chart appVersion for controller image tag" >&2
    exit 1
  fi

  if [[ "$use_local" == true ]]; then
    sed -E "s#image: ${driver_image_repository}:[^[:space:]]+#image: ${driver_image_repository}:${app_version}#g" \
      "$repo/csi-driver-for-windows-storage-server-controller.yaml" \
      | kubectl apply -f -
  else
    curl -fsSL "$repo/csi-driver-for-windows-storage-server-controller.yaml" \
      | sed -E "s#image: ${driver_image_repository}:[^[:space:]]+#image: ${driver_image_repository}:${app_version}#g" \
      | kubectl apply -f -
  fi
}

ensure_winrm_secret() {
  if [[ -z "${WINRM_HOST:-}" || -z "${WINRM_USER:-}" || -z "${WINRM_PASSWORD:-}" ]]; then
    if kubectl -n kube-system get secret csi-driver-winrm >/dev/null 2>&1; then
      return
    fi
    echo "missing Secret/kube-system/csi-driver-winrm and WINRM_HOST/WINRM_USER/WINRM_PASSWORD are not all set" >&2
    echo "create the secret manually or export those variables before running install-driver.sh" >&2
    exit 1
  fi
  secret_args=(
    --from-literal=WINRM_HOST="$WINRM_HOST"
    --from-literal=WINRM_PORT="${WINRM_PORT:-5986}"
    --from-literal=WINRM_TLS="${WINRM_TLS:-true}"
    --from-literal=WINRM_INSECURE="${WINRM_INSECURE:-true}"
    --from-literal=WINRM_AUTH="${WINRM_AUTH:-basic}"
    --from-literal=WINRM_TIMEOUT="${WINRM_TIMEOUT:-60s}"
    --from-literal=WINRM_USER="$WINRM_USER"
    --from-literal=WINRM_PASSWORD="$WINRM_PASSWORD"
  )
  if [[ -n "${WINRM_PS_IMPORT:-}" ]]; then
    secret_args+=(--from-literal=WINRM_PS_IMPORT="$WINRM_PS_IMPORT")
  fi
  kubectl -n kube-system create secret generic csi-driver-winrm \
    "${secret_args[@]}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

enable_nfs_kerberos() {
  echo "Enabling NFS Kerberos environment for NFS node daemonsets..."
  kubectl -n kube-system set env daemonset/csi-nfs-node -c nfs \
    KRB5_CONFIG=/host/etc/krb5.conf \
    KRB5_KTNAME=FILE:/host/etc/krb5.keytab \
    KRB5_CLIENT_KTNAME=FILE:/host/etc/krb5.keytab
  kubectl -n kube-system set env daemonset/csi-nfs-vhdx-node -c nfs-vhdx \
    KRB5_CONFIG=/host/etc/krb5.conf \
    KRB5_KTNAME=FILE:/host/etc/krb5.keytab \
    KRB5_CLIENT_KTNAME=FILE:/host/etc/krb5.keytab
  echo "NFS Kerberos enabled. Linux nodes must have Kerberos/NFS client configuration and credentials available on the host."
  echo "Use these StorageClass parameters for Kerberos-backed NFS volumes:"
  echo "  nfsAuthentication: \"$nfs_kerberos_flavor\""
  echo "  nfsMountAuthentication: \"$nfs_kerberos_flavor\""
}

echo "Installing Windows storage CSI drivers, version: $ver ..."
if [[ "$node_only" == true ]]; then
  echo "node-only mode enabled; skipping controller and WinRM setup"
  apply_manifest "csi-driver-for-windows-storage-server-driverinfo-nodeonly.yaml"
else
  ensure_winrm_secret
  apply_controller_manifest
  apply_manifest "csi-driver-for-windows-storage-server-driverinfo.yaml"
fi
apply_manifest "csi-nfs-for-windows-driverinfo.yaml"
apply_manifest "csi-nfs-vhdx-for-windows-driverinfo.yaml"
apply_manifest "csi-smb-for-windows-driverinfo.yaml"
apply_manifest "csi-smb-vhdx-for-windows-driverinfo.yaml"
apply_manifest "csi-driver-for-windows-storage-server-node.yaml"
apply_manifest "csi-nfs-for-windows-node.yaml"
apply_manifest "csi-nfs-vhdx-for-windows-node.yaml"
apply_manifest "csi-smb-for-windows-node.yaml"
apply_manifest "csi-smb-vhdx-for-windows-node.yaml"

if [[ "$nfs_kerberos" == true ]]; then
  enable_nfs_kerberos
fi

echo 'Windows storage CSI drivers installed successfully.'
