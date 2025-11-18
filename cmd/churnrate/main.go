// Command git-churnrate analyzes code churn in Git repositories.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/git-churnrate/pkg/churnrate"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	days := flag.Int("days", 28, "number of days to analyze")
	org := flag.String("org", "", "analyze top 10 most recently updated repos from a GitHub organization")
	flag.Parse()

	if *org != "" {
		return analyzeOrg(*org, *days)
	}

	repoPath := "."
	if flag.NArg() > 0 {
		repoPath = flag.Arg(0)
	}

	ctx := context.Background()
	m, err := churnrate.Analyze(ctx, repoPath, *days)
	if err != nil {
		return err
	}

	printReport(repoPath, m, *days)
	return nil
}

func analyzeOrg(org string, days int) error {
	repos, err := fetchOrgRepos(org)
	if err != nil {
		return fmt.Errorf("fetch repositories for org %s: %w", org, err)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no repositories found for organization: %s", org)
	}

	// Sort by most recently pushed
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].PushedAt.After(repos[j].PushedAt)
	})

	// Take top 10
	repos = repos[:min(10, len(repos))]

	fmt.Printf("\nAnalyzing top %d most recently updated repositories for %s\n", len(repos), org)
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	ctx := context.Background()
	var allMetrics []repoMetrics
	for i, repo := range repos {
		fmt.Printf("[%d/%d] %s\n", i+1, len(repos), repo.Name)
		m, err := churnrate.Analyze(ctx, repo.CloneURL, days)
		if err != nil {
			log.Printf("Warning: failed to analyze %s: %v", repo.Name, err)
			continue
		}
		allMetrics = append(allMetrics, repoMetrics{Name: repo.Name, Metrics: m})
		printReport(repo.Name, m, days)
		fmt.Println()
	}

	if len(allMetrics) > 0 {
		printOrgSummary(org, allMetrics, days)
	}

	return nil
}

type repoMetrics struct {
	Metrics *churnrate.Metrics
	Name    string
}

func printReport(name string, m *churnrate.Metrics, days int) {
	// Calculate excluded week dates if first week was excluded
	var excludedWeekStart, excludedWeekEnd time.Time
	if m.ExcludedFirstWeek {
		// Find the earliest week in churns (which would be after the excluded week)
		if len(m.Churns) > 0 {
			// The excluded week is one week before the earliest churn
			earliestWeek := m.Churns[0].Week
			for _, c := range m.Churns {
				if c.Week.Before(earliestWeek) {
					earliestWeek = c.Week
				}
			}
			excludedWeekEnd = earliestWeek.AddDate(0, 0, -1)
			excludedWeekStart = excludedWeekEnd.AddDate(0, 0, -6)
		}
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Git Repository Churn Analysis                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Repository:           %s\n", name)
	fmt.Printf("  Codebase Size:        %s lines\n", formatNumber(m.CodebaseSize))
	fmt.Printf("  Git Depth:            %d commits (earliest: %s)\n", m.GitDepth, m.EarliestCommit.Format("2006-01-02"))
	fmt.Printf("  Analysis Window:      %d days (%d weeks)\n", days, days/7)
	if !excludedWeekStart.IsZero() {
		fmt.Printf("  First Week Excluded:  %s to %s\n",
			excludedWeekStart.Format("2006-01-02"),
			excludedWeekEnd.Format("2006-01-02"))
	}
	fmt.Println()
	fmt.Println("  ─────────────────────────────────────────────────────────")
	fmt.Println()

	var total int
	for _, c := range m.Churns {
		total += c.Total()
	}

	fmt.Printf("  Total Churn:          %s lines changed\n", formatNumber(total))
	fmt.Printf("  Total Churn Rate:     %.2f%%\n", m.TotalChurnRate)
	fmt.Println()
	fmt.Printf("  Average Weekly Churn: %s lines/week\n", formatNumber(m.AvgWeeklyChurn))
	fmt.Printf("  Weekly Churn Rate:    %.2f%%\n", m.WeeklyChurnRate)
	fmt.Println()
	fmt.Println("  ─────────────────────────────────────────────────────────")
	fmt.Println()

	printTopChurnWeeks(m.Churns, 5)
	fmt.Println()
}

func printOrgSummary(org string, metrics []repoMetrics, days int) {
	var totalRate float64
	var totalSize int
	var totalAvg int

	for _, rm := range metrics {
		totalRate += rm.Metrics.WeeklyChurnRate
		totalSize += rm.Metrics.CodebaseSize
		totalAvg += rm.Metrics.AvgWeeklyChurn
	}

	avgRate := totalRate / float64(len(metrics))

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Printf("║          Organization Summary: %-28s║\n", org)
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Repositories Analyzed:       %d\n", len(metrics))
	fmt.Printf("  Total Codebase Size:         %s lines\n", formatNumber(totalSize))
	fmt.Printf("  Total Avg Weekly Churn:      %s lines/week\n", formatNumber(totalAvg))
	fmt.Printf("  Analysis Window:             %d days\n", days)
	fmt.Println()
	fmt.Println("  ─────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Printf("  Average Weekly Churn Rate:   %.2f%%\n", avgRate)
	fmt.Println()
	fmt.Println("  ─────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("  Individual Repository Churn Rates:")
	fmt.Println()

	// Sort by churn rate for display
	sorted := make([]repoMetrics, len(metrics))
	copy(sorted, metrics)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Metrics.WeeklyChurnRate > sorted[j].Metrics.WeeklyChurnRate
	})

	for _, rm := range sorted {
		fmt.Printf("    %-30s  %.2f%%/week\n", rm.Name, rm.Metrics.WeeklyChurnRate)
	}
	fmt.Println()
}

func printTopChurnWeeks(churns []churnrate.WeeklyChurn, limit int) {
	if len(churns) == 0 {
		return
	}

	sorted := make([]churnrate.WeeklyChurn, len(churns))
	copy(sorted, churns)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Total() > sorted[j].Total()
	})

	limit = min(limit, len(sorted))

	fmt.Printf("  Top %d Highest Churn Weeks:\n\n", limit)
	for i := range limit {
		c := sorted[i]
		fmt.Printf("    %s  +%s -%s  (%s total)\n",
			c.Week.Format("2006-01-02"),
			formatNumber(c.Additions),
			formatNumber(c.Deletions),
			formatNumber(c.Total()))
	}
}

func formatNumber(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}

	str := strconv.Itoa(n)
	var result strings.Builder

	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(digit)
	}

	return result.String()
}
