#!/usr/bin/env bash
# helm-test.sh — companion script for helm-deployment.yaml
#
# Runs three helm commands in sequence that the scenario intercepts:
#   1. helm repo add
#   2. helm upgrade --install
#   3. helm status
#
# Usage:
#   cli-replay exec examples/cookbook/helm-deployment.yaml -- bash examples/cookbook/helm-test.sh

set -euo pipefail

RELEASE_NAME="${RELEASE_NAME:-myrelease}"
CHART_REPO="bitnami"
CHART_NAME="nginx"

echo "==> Adding Helm repository..."
helm repo add "$CHART_REPO" "https://charts.bitnami.com/bitnami"

echo "==> Installing/upgrading release ${RELEASE_NAME}..."
helm upgrade --install "$RELEASE_NAME" "${CHART_REPO}/${CHART_NAME}"

echo "==> Checking release status..."
helm status "$RELEASE_NAME"

echo "==> Helm deployment complete."
