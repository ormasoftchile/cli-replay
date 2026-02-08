#!/bin/bash
# Companion test script for terraform-workflow.yaml
#
# Usage:
#   cli-replay exec examples/cookbook/terraform-workflow.yaml -- bash examples/cookbook/terraform-test.sh

set -euo pipefail

echo "=== Terraform Workflow Test ==="

# Step 1: Initialize
echo "Running: terraform init"
terraform init

# Step 2: Plan
echo "Running: terraform plan"
terraform plan -out=plan.tfplan

# Step 3: Apply
echo "Running: terraform apply"
terraform apply plan.tfplan

echo "=== Terraform Workflow Complete ==="
