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

usage() {
  cat <<EOF
Usage: $0 [version] [local] [--nfs-kerberos] [--nfs-kerberos-flavor krb5|krb5i|krb5p]

Examples:
  $0
  $0 v1.0.1
  $0 master local --nfs-kerberos --nfs-kerberos-flavor krb5p
EOF
}

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    local|--local)
      use_local=true
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

apply_manifest() {
  kubectl apply -f "$repo/$1"
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
apply_manifest "csi-driver-for-windows-storage-server-driverinfo.yaml"
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
