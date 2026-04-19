package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var (
	rootDir           string
	grepInvertMatch   bool
	grepCommitHash    string
	grepReferenceName string
	grepPathSpecs     string
	grepNamesOnly     bool
	grepPretty        bool
)

type grepResult struct {
	dir       string
	matches   []string
	fileNames map[string]int
}

var grepCmd = &cobra.Command{
	Use:   "grep",
	Short: "Run git grep in parallel across all git repos in cwd",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runGrep,
}

func init() {
	grepCmd.Flags().StringVarP(&rootDir, "directory", "C", "", "Root directory to search for git repos")
	grepCmd.Flags().BoolVarP(&grepInvertMatch, "invert-match", "v", false, "Select non-matching lines")
	grepCmd.Flags().StringVarP(&grepCommitHash, "commit", "c", "", "Commit hash to search in")
	grepCmd.Flags().StringVarP(&grepReferenceName, "branch", "b", "", "Branch or tag name to search in")
	grepCmd.Flags().StringVarP(&grepPathSpecs, "pathspec", "p", "", "Pathspec pattern to filter files")
	grepCmd.Flags().BoolVarP(&grepNamesOnly, "names-only", "", false, "Print only filenames with match count")
	grepCmd.Flags().BoolVarP(&grepPretty, "pretty", "", false, "Format output as ASCII table")
	rootCmd.AddCommand(grepCmd)
}

func runGrep(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	if rootDir == "" {
		rootDir = "."
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var wg sync.WaitGroup
	results := make(chan grepResult, 10)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		gitPath := filepath.Join(rootDir, entry.Name(), ".git")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue
		}

		wg.Add(1)
		go func(dir string) {
			defer wg.Done()

			repoPath := filepath.Join(rootDir, dir)
			repo, err := git.PlainOpen(repoPath)
			if err != nil {
				return
			}

			worktree, err := repo.Worktree()
			if err != nil {
				return
			}

			grepOpts := &git.GrepOptions{
				Patterns: []*regexp.Regexp{regexp.MustCompile(pattern)},
			}

			if grepInvertMatch {
				grepOpts.InvertMatch = true
			}
			if grepCommitHash != "" {
				hash := plumbing.NewHash(grepCommitHash)
				grepOpts.CommitHash = hash
			}
			if grepReferenceName != "" {
				grepOpts.ReferenceName = plumbing.ReferenceName(grepReferenceName)
			}
			if grepPathSpecs != "" {
				grepOpts.PathSpecs = []*regexp.Regexp{regexp.MustCompile(grepPathSpecs)}
			}

			grepResults, err := worktree.Grep(grepOpts)
			if err != nil {
				return
			}

			matches := make([]string, 0, len(grepResults))
			fileNames := make(map[string]int)
			for _, match := range grepResults {
				if grepNamesOnly {
					fileNames[match.FileName]++
				} else {
					highlighted := regexp.MustCompile("(?i)(" + pattern + ")").ReplaceAllString(match.Content, "\x1b[31m$1\x1b[0m")
					matches = append(matches, fmt.Sprintf("%s:%d:%s", match.FileName, match.LineNumber, highlighted))
				}
			}

			if len(matches) > 0 || len(fileNames) > 0 {
				results <- grepResult{dir: dir, matches: matches, fileNames: fileNames}
			}
		}(entry.Name())
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	if grepPretty && !grepNamesOnly {
		return fmt.Errorf("--pretty can only be used with --names-only")
	}

	if grepPretty && grepNamesOnly {
		var allResults []grepResult
		for result := range results {
			allResults = append(allResults, result)
		}
		printPrettyTable(allResults)
	} else {
		for result := range results {
			if grepNamesOnly {
				for fileName, count := range result.fileNames {
					fmt.Printf("\x1b[34m%s\x1b[0m:%s:%d\n", result.dir, fileName, count)
				}
			} else {
				for _, match := range result.matches {
					fmt.Printf("\x1b[34m%s\x1b[0m:%s\n", result.dir, match)
				}
			}
		}
	}

	return nil
}

func printPrettyTable(results []grepResult) {
	type tableRow struct{ repo, fileName, count string }
	rows := make([]tableRow, 0)
	for _, result := range results {
		for fileName, count := range result.fileNames {
			rows = append(rows, tableRow{result.dir, fileName, fmt.Sprintf("%d", count)})
		}
	}

	repoLen := 4
	fileNameLen := 8
	countLen := 5
	for _, r := range rows {
		if len(r.repo) > repoLen {
			repoLen = len(r.repo)
		}
		if len(r.fileName) > fileNameLen {
			fileNameLen = len(r.fileName)
		}
	}

	border := fmt.Sprintf("+%s+%s+%s+", strings.Repeat("-", repoLen), strings.Repeat("-", fileNameLen), strings.Repeat("-", countLen))
	header := fmt.Sprintf("| %-*s | %-*s | %-*s |", repoLen, "Repo", fileNameLen, "Filename", countLen, "Count")

	fmt.Println(border)
	fmt.Println(header)
	fmt.Println(border)
	for _, r := range rows {
		fmt.Printf("| %-*s | %-*s | %-*s |\n", repoLen, r.repo, fileNameLen, r.fileName, countLen, r.count)
	}
	fmt.Println(border)
}

func splitMatch(match string) []string {
	var parts []string
	var current string
	inColor := false
	for i := 0; i < len(match); i++ {
		if match[i] == '\x1b' && i+1 < len(match) && match[i+1] == '[' {
			inColor = true
			i++
			continue
		}
		if inColor && match[i] == 'm' {
			inColor = false
			continue
		}
		if match[i] == ':' && !inColor {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(match[i])
		}
	}
	parts = append(parts, current)
	return parts
}

