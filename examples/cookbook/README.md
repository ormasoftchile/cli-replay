# Scenario Cookbook

Copy-paste-ready scenarios for common CLI workflow patterns. Each example includes an annotated scenario YAML and a companion test script.

## Decision Matrix

| Use Case | Example | Key Features | Complexity |
|----------|---------|-------------|------------|
| IaC provisioning pipeline | [Terraform Workflow](terraform-workflow.yaml) | Linear steps, multi-line JSON output, exit codes | Simple |
| Package/release management | [Helm Deployment](helm-deployment.yaml) | Captures, template variables, realistic output | Medium |
| Kubernetes deployment + monitoring | [kubectl Pipeline](kubectl-pipeline.yaml) | Step groups, call bounds, dynamic captures | Advanced |

## Quick Start

```bash
# 1. Validate any cookbook scenario
cli-replay validate examples/cookbook/terraform-workflow.yaml

# 2. Run with the companion test script
cli-replay exec examples/cookbook/terraform-workflow.yaml -- bash examples/cookbook/terraform-test.sh

# 3. Check the result
echo $?  # 0 = all steps consumed
```

## Examples

### Terraform Workflow

**File**: [terraform-workflow.yaml](terraform-workflow.yaml)  
**Script**: [terraform-test.sh](terraform-test.sh)

A linear `terraform init` → `terraform plan` → `terraform apply` pipeline. Demonstrates:
- Sequential step matching
- Multi-line JSON stdout (realistic Terraform output)
- Non-zero exit codes (plan with changes)
- Security allowlist (`allowed_commands: ["terraform"]`)

### Helm Deployment

**File**: [helm-deployment.yaml](helm-deployment.yaml)  
**Script**: [helm-test.sh](helm-test.sh)

A `helm repo add` → `helm upgrade --install` → `helm status` workflow. Demonstrates:
- Template variables (`meta.vars`)
- Capture and reuse across steps (`respond.capture`)
- Realistic multi-line output
- Security allowlist

### kubectl Pipeline

**File**: [kubectl-pipeline.yaml](kubectl-pipeline.yaml)  
**Script**: [kubectl-test.sh](kubectl-test.sh)

A multi-step kubectl deployment with monitoring. Demonstrates:
- Step groups with `mode: unordered` for parallel-safe checks
- Call count bounds (`calls.min`/`calls.max`) for polling steps
- Dynamic captures referenced in later steps
- Full-featured scenario combining all major cli-replay features

## Tips

- Run `cli-replay validate` before `exec` to catch YAML errors early
- Use `--dry-run` to preview the step sequence without side effects
- Add `meta.security.allowed_commands` to every production scenario
- Store cookbook scenarios in your repo's `examples/` directory
