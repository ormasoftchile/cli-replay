# Benchmarks

Baseline performance numbers for cli-replay's hot paths. Use these to detect regressions during development.

## Running Benchmarks

```bash
go test -run=XXX -bench=. -benchmem -count=5 \
  ./internal/matcher/ \
  ./internal/runner/ \
  ./internal/verify/
```

> **Note (Windows/PowerShell):** Quote the `-bench` flag: `'-bench=.'`

### Comparing Against Baseline

Install [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat):

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Save current results and compare:

```bash
go test -run=XXX -bench=. -benchmem -count=5 ./internal/matcher/ ./internal/runner/ ./internal/verify/ > new.txt
benchstat baseline.txt new.txt
```

## Reference System

| Property    | Value                                |
|-------------|--------------------------------------|
| OS          | Windows (amd64)                      |
| CPU         | AMD EPYC 7763 64-Core Processor      |
| GOMAXPROCS  | 16                                   |
| Go version  | 1.21+                                |

## Baseline Results

### Matcher (`internal/matcher/`)

| Benchmark                       | ns/op | B/op | allocs/op |
|---------------------------------|------:|-----:|----------:|
| BenchmarkArgvMatch/steps=100    | 2,448 |    0 |         0 |
| BenchmarkArgvMatch/steps=500    | 12,136 |   0 |         0 |
| BenchmarkGroupMatch_50          | 1,196 |    0 |         0 |

**Key observation:** Matcher is zero-allocation — pure comparison logic with no heap pressure.

### Runner (`internal/runner/`)

| Benchmark                        |   ns/op |  B/op | allocs/op |
|----------------------------------|--------:|------:|----------:|
| BenchmarkStateRoundTrip/steps=100 | 3,112,000 | 9,620 |       48 |
| BenchmarkStateRoundTrip/steps=500 | 2,918,000 | 28,448 |      51 |
| BenchmarkReplayOrchestration_100  |  393,000 | 384,305 |   3,503 |

**Key observation:** StateRoundTrip is I/O-bound (YAML serialization to disk). ReplayOrchestration exercises the full replay pipeline in memory.

### Verify (`internal/verify/`)

| Benchmark                |   ns/op |    B/op | allocs/op |
|--------------------------|--------:|--------:|----------:|
| BenchmarkFormatJSON      |   2,892 |       0 |         0 |
| BenchmarkFormatJUnit     |  25,117 |  11,400 |        86 |
| BenchmarkBuildResult     |   1,949 |   1,376 |        15 |
| BenchmarkFormatJSON_200  |  54,065 |       0 |         0 |
| BenchmarkFormatJUnit_200 | 372,410 | 107,350 |     1,341 |

**Key observation:** JSON output is nearly zero-allocation. JUnit XML generation allocates proportionally with step count due to XML element construction.

## Regression Thresholds

| Level     | Multiplier | Action Required                          |
|-----------|:----------:|------------------------------------------|
| OK        |    ≤ 1.5×  | No action needed                         |
| Warning   |    ≤ 2×    | Investigate; document if intentional      |
| Regression|    > 2×    | Must fix or justify before merge         |
| Critical  |    > 4×    | Block merge — requires resolution        |

A change is considered a **regression** if `benchstat` reports a statistically significant increase (p < 0.05) exceeding the threshold above.

## Contributing Benchmarks

### Naming Convention

```
Benchmark<Area>_<Scale>
```

Examples:
- `BenchmarkArgvMatch` — core matching logic
- `BenchmarkStateRoundTrip/steps=100` — parameterized by scale
- `BenchmarkFormatJUnit_200` — 200-step JUnit generation

### Adding a New Benchmark

1. Create the benchmark function in `*_bench_test.go` (or `bench_test.go`) alongside existing tests
2. Follow the naming convention above
3. Run with `-count=5` and record median values
4. Add a row to the appropriate table in this file
5. Commit the updated baseline

### When to Update Baselines

- After intentional performance changes (algorithm improvements, data structure changes)
- After significant dependency updates
- When changing the reference system
- **Do not** update baselines to mask regressions

## CI Integration Guidance

To add benchmark regression checks to a CI pipeline:

```yaml
# GitHub Actions example
- name: Run benchmarks
  run: |
    go test -run=XXX -bench=. -benchmem -count=5 \
      ./internal/matcher/ ./internal/runner/ ./internal/verify/ \
      | tee bench-current.txt

- name: Compare with baseline
  run: |
    go install golang.org/x/perf/cmd/benchstat@latest
    benchstat bench-baseline.txt bench-current.txt
```

Store `bench-baseline.txt` in the repository and update it with each release.
