# cli-replay Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-07

## Active Technologies
- Go 1.21 + cobra v1.8.0, yaml.v3, testify v1.8.4 (005-p0-critical-enhancements)
- JSON state files in `os.TempDir()` (SHA256-hashed filenames) (005-p0-critical-enhancements)
- Go 1.21 + cobra v1.8.0 (CLI framework), yaml.v3 (scenario loading), testify v1.8.4 (testing), golang.org/x/term v0.18.0 (006-p1-cicd-enhancements)
- JSON state files in `os.TempDir()` (SHA256-hashed filenames via `StateFilePathWithSession`) (006-p1-cicd-enhancements)
- Go 1.21+ (module: `github.com/cli-replay/cli-replay`) + cobra v1.8.0, gopkg.in/yaml.v3 v3.0.1, testify v1.8.4, golang.org/x/term v0.18.0 (007-p2-quality-of-life)
- JSON state files on disk in `.cli-replay/` adjacent to scenario file (007-p2-quality-of-life)

- (005-p0-critical-enhancements)

## Project Structure

```text
backend/
frontend/
tests/
```

## Commands

# Add commands for 

## Code Style

: Follow standard conventions

## Recent Changes
- 007-p2-quality-of-life: Added Go 1.21+ (module: `github.com/cli-replay/cli-replay`) + cobra v1.8.0, gopkg.in/yaml.v3 v3.0.1, testify v1.8.4, golang.org/x/term v0.18.0
- 006-p1-cicd-enhancements: Added Go 1.21 + cobra v1.8.0 (CLI framework), yaml.v3 (scenario loading), testify v1.8.4 (testing), golang.org/x/term v0.18.0
- 005-p0-critical-enhancements: Added Go 1.21 + cobra v1.8.0, yaml.v3, testify v1.8.4


<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
