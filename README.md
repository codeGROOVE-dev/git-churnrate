# git-churnrate

Analyze code churn in Git repositories to measure codebase stability and change velocity.

## Installation

```bash
go install github.com/codeGROOVE-dev/git-churnrate/cmd/churnrate@latest
```

## Usage

### Single Repository Analysis

Analyze a local repository:

```bash
churnrate /path/to/repo
```

Analyze a remote repository:

```bash
churnrate https://github.com/owner/repo.git
```

Specify analysis window (default: 28 days):

```bash
churnrate --days 60 https://github.com/owner/repo.git
```

### Organization Analysis

Analyze the 10 most recently updated repositories from a GitHub organization:

```bash
churnrate --org kubernetes --days 30
```

## Metrics

- **Weekly Churn Rate**: Average lines changed per week as percentage of codebase size
- **Total Churn Rate**: Total lines changed over analysis period as percentage of codebase
- **Codebase Size**: Total lines in tracked files
- **Git Depth**: Number of commits fetched (days × 35)

## Algorithm

1. Clone repository with shallow depth (35 commits per day analyzed)
2. Count lines in all tracked files
3. Aggregate additions/deletions by ISO week
4. Exclude first week of repository history (bootstrap commits)
5. Calculate churn rates and statistics

## Implementation

Written in Go following stdlib conventions. Core analysis in `pkg/churnrate` package for programmatic use.

```go
import "github.com/codeGROOVE-dev/git-churnrate/pkg/churnrate"

ctx := context.Background()
metrics, err := churnrate.Analyze(ctx, "https://github.com/owner/repo.git", 28)
```

## License

Apache 2.0
