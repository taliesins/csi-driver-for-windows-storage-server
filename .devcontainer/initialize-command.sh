#!/bin/bash
set -ex

echo "Running initialize command script..."
# This script is run when the devcontainer is created, but before the post-create command.

while getopts w: flag; do
  case "${flag}" in
    w) local_workspace_path=${OPTARG} ;;
    *) throw 'Unknown argument' ;;
  esac
done

echo "local_workspace_path=${local_workspace_path}"
