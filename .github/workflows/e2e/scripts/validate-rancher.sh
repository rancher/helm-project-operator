#!/bin/bash
set -e

sleep "${DEFAULT_SLEEP_TIMEOUT_SECONDS}"
# Check deployment status
kubectl -n cattle-system rollout status deploy/rancher

# Capture the exit status of the previous command
exit_status=$?

if [ $exit_status -eq 0 ]; then
    echo "rancher deployment is healthy."
else
    echo "rancher deployment is not healthy."
fi

exit $exit_status
