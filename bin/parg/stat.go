package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	statDirectory   string
	statSince       string
	statUntil       string
	statBranch      string
	statBy          string
	statNoMerges    bool
	statSort        string
	statPretty      bool
	statParallelism int
	statVerbose     bool
	statNoStats     bool
)

func statLogf(format string, args ...any) {
	if !statVerbose {
		return
	}
	fmt.Fprintf(os.Stderr, "[stat %s] %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}

type authorStats struct {
	Name    string
	Email   string
	Commits int
	Files   int
	Added   int
	Deleted int
	Repos   map[string]struct{}
}

type repoResult struct {
	dir    string
	byline map[string]*authorStats
	err    error
}

type commitInfo struct {
	Author  string
	Email   string
	Files   int
	Added   int
	Deleted int
}

var statCmd = &cobra.Command{
	Use:   "stat",
	Short: "Summarize per-author commits/files/lines across all git repos in cwd",
	RunE:  runStat,
}

func init() {
	statCmd.Flags().StringVarP(&statDirectory, "directory", "C", "", "Root directory to search for git repos")
	statCmd.Flags().StringVar(&statSince, "since", "", "Only include commits after this date (RFC3339 or YYYY-MM-DD)")
	statCmd.Flags().StringVar(&statUntil, "until", "", "Only include commits before this date (RFC3339 or YYYY-MM-DD)")
	statCmd.Flags().StringVarP(&statBranch, "branch", "b", "", "Branch/ref to scan (default: current HEAD)")
	statCmd.Flags().StringVar(&statBy, "by", "author", "Group by 'author' or 'committer'")
	statCmd.Flags().BoolVar(&statNoMerges, "no-merges", false, "Skip merge commits")
	statCmd.Flags().StringVar(&statSort, "sort", "commits", "Sort by 'commits' (default), 'files', 'added', 'deleted', or 'name'")
	statCmd.Flags().BoolVar(&statPretty, "pretty", false, "Format output as an ASCII table")
	statCmd.Flags().IntVarP(&statParallelism, "parallelism", "c", 8, "Number of repos to scan in parallel")
	statCmd.Flags().BoolVarP(&statVerbose, "verbose", "v", false, "Log per-repo progress to stderr")
	statCmd.Flags().BoolVar(&statNoStats, "no-stats", false, "Skip per-file diff stats (commits only, much faster)")
	rootCmd.AddCommand(statCmd)
}

func parseStatDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	layouts := []string{time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid date %q (expected RFC3339 or YYYY-MM-DD)", s)
}

func authorKey(name, email string) string {
	return fmt.Sprintf("%s <%s>", name, email)
}

func runGitLog(repoPath, ref string, since, until *time.Time, noMerges, noStats, byCommitter bool) ([]commitInfo, []byte, error) {
	format := "PARG-COMMIT%x00%H%x00"
	if byCommitter {
		format += "%cn%x00%ce%x00"
	} else {
		format += "%an%x00%ae%x00"
	}

	args := []string{"--no-pager", "log", "--pretty=tformat:" + format}
	if !noStats {
		args = append(args, "-M", "--numstat")
	}
	if ref != "" {
		args = append(args, ref)
	}
	if noMerges {
		args = append(args, "--no-merges")
	}
	if since != nil {
		args = append(args, "--since="+since.Format(time.RFC3339))
	}
	if until != nil {
		args = append(args, "--until="+until.Format(time.RFC3339))
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return parseGitLogOutput(stdout.Bytes(), noStats), stderr.Bytes(), err
}

func parseGitLogOutput(data []byte, noStats bool) []commitInfo {
	var commits []commitInfo
	var current *commitInfo

	scanner := bufio.NewScanner(bytes.NewReader(data))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)

	commitPrefix := []byte("PARG-COMMIT\x00")
	tab := []byte("\t")
	nullSep := []byte("\x00")

	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.HasPrefix(line, commitPrefix) {
			rest := line[len(commitPrefix):]
			parts := bytes.SplitN(rest, nullSep, 4)
			if len(parts) < 4 {
				continue
			}
			if current != nil {
				commits = append(commits, *current)
			}
			current = &commitInfo{
				Author: string(parts[1]),
				Email:  string(parts[2]),
			}
			continue
		}
		if current == nil || noStats {
			continue
		}
		parts := bytes.SplitN(line, tab, 3)
		if len(parts) != 3 {
			continue
		}
		add, errAdd := strconv.Atoi(string(parts[0]))
		del, errDel := strconv.Atoi(string(parts[1]))
		if errAdd != nil || errDel != nil {
			continue
		}
		current.Files++
		current.Added += add
		current.Deleted += del
	}
	if current != nil {
		commits = append(commits, *current)
	}
	return commits
}

func runStat(cmd *cobra.Command, args []string) error {
	if statDirectory == "" {
		statDirectory = "."
	}

	switch statBy {
	case "author", "committer":
	default:
		return fmt.Errorf("--by must be 'author' or 'committer'")
	}

	switch statSort {
	case "commits", "files", "added", "deleted", "name":
	default:
		return fmt.Errorf("--sort must be one of: commits, files, added, deleted, name")
	}

	since, err := parseStatDate(statSince)
	if err != nil {
		return err
	}
	until, err := parseStatDate(statUntil)
	if err != nil {
		return err
	}

	if statParallelism <= 0 {
		statParallelism = 8
	}

	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git binary not found in PATH (required by stat)")
	}

	entries, err := os.ReadDir(statDirectory)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	type repoEntry struct {
		name string
		path string
	}
	var repos []repoEntry

	if _, err := os.Stat(filepath.Join(statDirectory, ".git")); err == nil {
		abs, _ := filepath.Abs(statDirectory)
		name := filepath.Base(abs)
		if name == "" || name == "." || name == string(filepath.Separator) {
			name = filepath.Base(statDirectory)
		}
		repos = append(repos, repoEntry{name: name, path: statDirectory})
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		gitPath := filepath.Join(statDirectory, entry.Name(), ".git")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue
		}
		repos = append(repos, repoEntry{
			name: entry.Name(),
			path: filepath.Join(statDirectory, entry.Name()),
		})
	}

	if len(repos) == 0 {
		return fmt.Errorf("no git repos found in %s", statDirectory)
	}

	statLogf("scanning %d repos in %s (parallelism=%d, no-stats=%t)", len(repos), statDirectory, statParallelism, statNoStats)

	sem := make(chan struct{}, statParallelism)
	results := make(chan repoResult, len(repos))
	var wg sync.WaitGroup

	for _, repo := range repos {
		wg.Add(1)
		go func(entry repoEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			statLogf("scan: %s", entry.name)

			rr := repoResult{dir: entry.name, byline: map[string]*authorStats{}}

			commits, gitStderr, err := runGitLog(entry.path, statBranch, since, until, statNoMerges, statNoStats, statBy == "committer")
			if err != nil {
				rr.err = err
				if statVerbose && len(gitStderr) > 0 {
					statLogf("  %s: git stderr: %s", entry.name, strings.TrimSpace(string(gitStderr)))
				}
				statLogf("FAIL %s in %s: %v", entry.name, time.Since(start), err)
				results <- rr
				return
			}

			statLogf("  %s: %d commits in %s", entry.name, len(commits), time.Since(start))

			for _, c := range commits {
				key := authorKey(c.Author, c.Email)
				as, ok := rr.byline[key]
				if !ok {
					as = &authorStats{
						Name:  c.Author,
						Email: c.Email,
						Repos: map[string]struct{}{},
					}
					rr.byline[key] = as
				}
				as.Commits++
				as.Files += c.Files
				as.Added += c.Added
				as.Deleted += c.Deleted
				as.Repos[entry.name] = struct{}{}
			}

			statLogf("done %s: %d commits, %d authors in %s", entry.name, len(commits), len(rr.byline), time.Since(start))
			results <- rr
		}(repo)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	merged := map[string]*authorStats{}
	for rr := range results {
		if rr.err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", rr.dir, rr.err)
			continue
		}
		if len(rr.byline) == 0 {
			continue
		}
		for key, src := range rr.byline {
			dst, ok := merged[key]
			if !ok {
				dst = &authorStats{
					Name:  src.Name,
					Email: src.Email,
					Repos: map[string]struct{}{},
				}
				merged[key] = dst
			}
			dst.Commits += src.Commits
			dst.Files += src.Files
			dst.Added += src.Added
			dst.Deleted += src.Deleted
			for repo := range src.Repos {
				dst.Repos[repo] = struct{}{}
			}
		}
	}

	if len(merged) == 0 {
		fmt.Fprintln(os.Stderr, "no commits found")
		return nil
	}

	authors := make([]*authorStats, 0, len(merged))
	for _, a := range merged {
		authors = append(authors, a)
	}
	sortAuthors(authors, statSort)

	if statPretty {
		printStatTable(os.Stdout, authors)
	} else {
		printStatPlain(os.Stdout, authors)
	}
	return nil
}

func sortAuthors(authors []*authorStats, key string) {
	sort.Slice(authors, func(i, j int) bool {
		a, b := authors[i], authors[j]
		switch key {
		case "name":
			na := strings.ToLower(a.Name)
			nb := strings.ToLower(b.Name)
			if na != nb {
				return na < nb
			}
			return a.Email < b.Email
		case "files":
			if a.Files != b.Files {
				return a.Files > b.Files
			}
		case "added":
			if a.Added != b.Added {
				return a.Added > b.Added
			}
		case "deleted":
			if a.Deleted != b.Deleted {
				return a.Deleted > b.Deleted
			}
		}
		if a.Commits != b.Commits {
			return a.Commits > b.Commits
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

func printStatPlain(w io.Writer, authors []*authorStats) {
	for _, a := range authors {
		fmt.Fprintf(w, "%s <%s>: %d commits, %d files, +%d -%d [%s]\n",
			a.Name, a.Email, a.Commits, a.Files, a.Added, a.Deleted, repoList(a))
	}
}

type statRow struct {
	cells []string
}

func printStatTable(w io.Writer, authors []*authorStats) {
	headers := []string{"Author", "Commits", "Files", "Added", "Deleted", "Repos"}
	rows := make([]statRow, 0, len(authors))
	for _, a := range authors {
		rows = append(rows, statRow{
			cells: []string{
				fmt.Sprintf("%s <%s>", a.Name, a.Email),
				fmt.Sprintf("%d", a.Commits),
				fmt.Sprintf("%d", a.Files),
				fmt.Sprintf("+%d", a.Added),
				fmt.Sprintf("-%d", a.Deleted),
				repoList(a),
			},
		})
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r.cells {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	border := "+" + strings.Join(repeatDashes(widths), "+") + "+"
	fmt.Fprintln(w, border)
	fmt.Fprintf(w, "| %s |\n", joinRow(headers, widths))
	fmt.Fprintln(w, border)
	for _, r := range rows {
		fmt.Fprintf(w, "| %s |\n", joinRow(r.cells, widths))
	}
	fmt.Fprintln(w, border)
}

func repeatDashes(widths []int) []string {
	out := make([]string, len(widths))
	for i, w := range widths {
		out[i] = strings.Repeat("-", w+2)
	}
	return out
}

func joinRow(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = fmt.Sprintf(" %-*s ", widths[i], c)
	}
	return strings.Join(parts, "|")
}

func repoList(a *authorStats) string {
	if len(a.Repos) == 0 {
		return ""
	}
	names := make([]string, 0, len(a.Repos))
	for r := range a.Repos {
		names = append(names, r)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}