#!/bin/bash
set -ex
echo "Running post-create script..."
# This script is executed after the container is created and the workspace is initialized.

while getopts w: flag; do
  case "${flag}" in
    w) local_workspace_path=${OPTARG} ;;
    *) throw 'Unknown argument' ;;
  esac
done

echo "local_workspace_path=${local_workspace_path}"

#git config --global --add safe.directory /workspace
