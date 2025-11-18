# git-churnrate

Analyze code churn in Git repositories to measure codebase stability.

## Installation

```bash
go install github.com/codeGROOVE-dev/git-churnrate/cmd/churnrate@latest
```

## Usage

```bash
churnrate https://github.com/owner/repo.git
churnrate --days 60 /path/to/repo
churnrate --org kubernetes --days 30
```

## Example Output

```
  Repository:           https://github.com/chainguard-dev/apko.git
  Codebase Size:        51,592 lines
  Git Depth:            980 commits (earliest: 2022-02-28)
  Analysis Window:      28 days (4 weeks)

  Total Churn:          688 lines changed
  Total Churn Rate:     1.33%
  Average Weekly Churn: 172 lines/week
  Weekly Churn Rate:    0.33%

  Top 4 Highest Churn Weeks:
    2025-11-03  +155 -167  (322 total)
    2025-11-10  +145 -46  (191 total)
    2025-10-27  +75 -72  (147 total)
    2025-11-17  +20 -8  (28 total)
```

## Metrics

- **Weekly Churn Rate**: Average lines changed per week as percentage of codebase
- **Total Churn Rate**: Total lines changed as percentage of codebase
- **Git Depth**: Shallow clone depth (days × 35 commits)

## Algorithm

1. Shallow clone repository (35 commits per day)
2. Count lines in tracked files
3. Aggregate additions/deletions by ISO week
4. Exclude first week (bootstrap commits)
5. Calculate churn statistics

## Programmatic Use

```go
import "github.com/codeGROOVE-dev/git-churnrate/pkg/churnrate"

ctx := context.Background()
metrics, err := churnrate.Analyze(ctx, "https://github.com/owner/repo.git", 28)
```

## License

Apache 2.0
