#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
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

usage() {
  cat <<EOF
Usage: $0 [version] [local] [--nfs-kerberos] [--nfs-kerberos-flavor krb5|krb5i|krb5p]

Kerberos flags are accepted so install and uninstall commands can use the same
argument set; uninstall deletes the daemonsets and does not need separate
Kerberos cleanup.
EOF
}

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    local|--local)
      use_local=true
      ;;
    --nfs-kerberos|--kerberos)
      ;;
    --nfs-kerberos-flavor|--kerberos-flavor)
      if [[ "$#" -lt 2 ]]; then
        echo "missing value for $1" >&2
        usage >&2
        exit 1
      fi
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

repo="https://raw.githubusercontent.com/taliesins/csi-driver-for-windows-storage-server/$ver/deploy"
if [[ "$use_local" == true ]]; then
  echo "use local deploy"
  repo="./deploy"
fi

delete_manifest() {
  kubectl delete -f "$repo/$1" --ignore-not-found
}

echo "Uninstalling Windows storage CSI drivers, version: $ver ..."
delete_manifest "csi-smb-vhdx-for-windows-node.yaml"
delete_manifest "csi-smb-for-windows-node.yaml"
delete_manifest "csi-nfs-vhdx-for-windows-node.yaml"
delete_manifest "csi-nfs-for-windows-node.yaml"
delete_manifest "csi-driver-for-windows-storage-server-node.yaml"
delete_manifest "csi-smb-vhdx-for-windows-driverinfo.yaml"
delete_manifest "csi-smb-for-windows-driverinfo.yaml"
delete_manifest "csi-nfs-vhdx-for-windows-driverinfo.yaml"
delete_manifest "csi-nfs-for-windows-driverinfo.yaml"
delete_manifest "csi-driver-for-windows-storage-server-driverinfo.yaml"
echo 'Windows storage CSI drivers uninstalled successfully.'
