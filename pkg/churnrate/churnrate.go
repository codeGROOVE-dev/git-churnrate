// Package churnrate provides Git repository churn analysis capabilities.
package churnrate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WeeklyChurn represents code churn statistics for a single week.
type WeeklyChurn struct {
	Week      time.Time
	Additions int
	Deletions int
}

// Total returns the total lines changed (additions + deletions).
func (w WeeklyChurn) Total() int {
	return w.Additions + w.Deletions
}

// Metrics contains the churn analysis results for a repository.
type Metrics struct {
	EarliestCommit    time.Time
	Churns            []WeeklyChurn
	WeeklyChurnRate   float64
	TotalChurnRate    float64
	CodebaseSize      int
	AvgWeeklyChurn    int
	GitDepth          int
	ExcludedFirstWeek bool
}

// Analyze performs churn analysis on a Git repository over the specified number of days.
// The path can be either a local directory or a Git URL (http://, https://, git@, git://).
func Analyze(ctx context.Context, path string, days int) (*Metrics, error) {
	var absPath string
	var cleanup func()
	var err error

	// Check if this is a Git URL
	isURL := strings.HasPrefix(path, "http://") ||
		strings.HasPrefix(path, "https://") ||
		strings.HasPrefix(path, "git@") ||
		strings.HasPrefix(path, "git://")

	if isURL {
		absPath, cleanup, err = cloneRepo(ctx, path, days)
		if err != nil {
			return nil, fmt.Errorf("clone repository: %w", err)
		}
		defer cleanup()
	} else {
		absPath, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve path: %w", err)
		}

		// Check if directory has .git subdirectory
		gitDir := filepath.Join(absPath, ".git")
		info, err := os.Stat(gitDir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("not a git repository: %s", absPath)
		}
	}

	size, err := codebaseSize(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("calculate codebase size: %w", err)
	}
	if size == 0 {
		return nil, errors.New("repository has no tracked files")
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	firstCommit, firstWeekEnd, err := firstWeekInfo(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("calculate first week: %w", err)
	}

	churns, err := weeklyChurns(ctx, absPath, since, firstWeekEnd)
	if err != nil {
		return nil, fmt.Errorf("calculate weekly churns: %w", err)
	}

	if len(churns) == 0 {
		return nil, fmt.Errorf("no commit history found in the last %d days", days)
	}

	excludedFirstWeek := firstWeekEnd.After(since)

	var total int
	for _, c := range churns {
		total += c.Total()
	}

	avg := float64(total) / float64(len(churns))
	weeklyRate := (avg / float64(size)) * 100
	totalRate := (float64(total) / float64(size)) * 100

	return &Metrics{
		CodebaseSize:      size,
		WeeklyChurnRate:   weeklyRate,
		TotalChurnRate:    totalRate,
		AvgWeeklyChurn:    int(avg + 0.5),
		EarliestCommit:    firstCommit,
		GitDepth:          days * 35,
		ExcludedFirstWeek: excludedFirstWeek,
		Churns:            churns,
	}, nil
}

func cloneRepo(ctx context.Context, url string, days int) (path string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "churn-*")
	if err != nil {
		return "", nil, err
	}

	cleanup = func() {
		_ = os.RemoveAll(tmpDir) //nolint:errcheck // Best effort cleanup
	}

	depth := days * 35
	depthArg := fmt.Sprintf("--depth=%d", depth)
	cmd := exec.CommandContext(ctx, "git", "clone", depthArg, url, tmpDir)
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmpDir, cleanup, nil
}

func codebaseSize(ctx context.Context, repoPath string) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var total int
	files := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, fname := range files {
		if fname == "" {
			continue
		}

		path := filepath.Join(repoPath, fname)
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		var n int
		s := bufio.NewScanner(f)
		for s.Scan() {
			n++
		}
		_ = f.Close() //nolint:errcheck // Read-only file close

		if s.Err() != nil {
			continue
		}
		total += n
	}

	return total, nil
}

func firstWeekInfo(ctx context.Context, repoPath string) (firstCommit, firstWeekEnd time.Time, err error) {
	cmd := exec.CommandContext(ctx, "git", "log", "--all", "--pretty=format:%ct")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return time.Time{}, time.Time{}, errors.New("no commits found")
	}

	// The last line is the oldest commit
	timestampStr := strings.TrimSpace(lines[len(lines)-1])
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	firstCommit = time.Unix(timestamp, 0).UTC()
	year, week := firstCommit.ISOWeek()
	firstWeekStart := isoWeekStart(year, week)
	firstWeekEnd = firstWeekStart.AddDate(0, 0, 7)
	return firstCommit, firstWeekEnd, nil
}

func weeklyChurns(ctx context.Context, repoPath string, since, firstWeekEnd time.Time) ([]WeeklyChurn, error) {
	sinceArg := fmt.Sprintf("--since=%s", since.Format(time.RFC3339))
	cmd := exec.CommandContext(ctx, "git", "log", "--all", "--numstat", "--pretty=format:%ct", sinceArg)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	weeklyData := make(map[string]*WeeklyChurn)
	var currentWeek string
	var currentTime time.Time

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if timestamp, err := strconv.ParseInt(line, 10, 64); err == nil {
			t := time.Unix(timestamp, 0).UTC()
			if t.Before(since) {
				currentWeek = ""
				continue
			}

			currentTime = t
			year, week := t.ISOWeek()
			currentWeek = fmt.Sprintf("%d-W%02d", year, week)

			if _, exists := weeklyData[currentWeek]; !exists {
				weekStart := isoWeekStart(year, week)
				weeklyData[currentWeek] = &WeeklyChurn{Week: weekStart}
			}
			continue
		}

		if currentWeek == "" || currentTime.Before(since) {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 3 {
			continue
		}

		additions, err1 := strconv.Atoi(parts[0])
		deletions, err2 := strconv.Atoi(parts[1])

		if err1 != nil || err2 != nil {
			continue
		}

		churn := weeklyData[currentWeek]
		churn.Additions += additions
		churn.Deletions += deletions
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var churns []WeeklyChurn
	for _, churn := range weeklyData {
		// Only exclude first week if it falls within our analysis window
		if churn.Week.Before(firstWeekEnd) && firstWeekEnd.After(since) {
			continue
		}
		churns = append(churns, *churn)
	}

	return churns, nil
}

func isoWeekStart(year, week int) time.Time {
	t := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)

	if wd := t.Weekday(); wd <= time.Thursday {
		t = t.AddDate(0, 0, -int(wd)+1)
	} else {
		t = t.AddDate(0, 0, 8-int(wd))
	}

	_, w := t.ISOWeek()
	t = t.AddDate(0, 0, (week-w)*7)

	return t
}
