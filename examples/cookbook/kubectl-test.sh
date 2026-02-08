#!/usr/bin/env bash
# kubectl-test.sh — companion script for kubectl-pipeline.yaml
#
# Runs a realistic kubectl deployment pipeline:
#   1. Apply manifests
#   2. Set new image version
#   3. Wait for rollout (poll)
#   4. Post-deploy checks (get pods, logs, describe — any order)
#
# Usage:
#   cli-replay exec examples/cookbook/kubectl-pipeline.yaml -- bash examples/cookbook/kubectl-test.sh

set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
DEPLOYMENT="${DEPLOYMENT:-myapp}"
IMAGE="${IMAGE:-myapp:v2.1.0}"

echo "==> Applying manifests..."
kubectl apply -f k8s/deployment.yaml -n "$NAMESPACE"

echo "==> Setting image to ${IMAGE}..."
kubectl set image "deployment/${DEPLOYMENT}" "${DEPLOYMENT}=${IMAGE}" -n "$NAMESPACE"

echo "==> Waiting for rollout..."
kubectl rollout status "deployment/${DEPLOYMENT}" -n "$NAMESPACE"

echo "==> Fetching pod list..."
kubectl get pods -l "app=${DEPLOYMENT}" -n "$NAMESPACE" -o json

echo "==> Fetching logs from first pod..."
kubectl logs "${DEPLOYMENT}-7d4b8c6f5-abc12" -n "$NAMESPACE" --tail=20

echo "==> Describing deployment..."
kubectl describe deployment "$DEPLOYMENT" -n "$NAMESPACE"

echo "==> Deployment pipeline complete."
