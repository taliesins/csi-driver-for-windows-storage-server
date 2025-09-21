#!/bin/bash
set -ex

echo "Running post-start script..."
# This script is run after the container starts, but before the user connects to it.

while getopts w: flag; do
  case "${flag}" in
    w) local_workspace_path=${OPTARG} ;;
    *) throw 'Unknown argument' ;;
  esac
done

echo "local_workspace_path=${local_workspace_path}"
