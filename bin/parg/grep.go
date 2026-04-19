package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var (
	rootDir          string
	grepInvertMatch  bool
	grepCommitHash   string
	grepReferenceName string
	grepPathSpecs    string
)

type grepResult struct {
	dir     string
	matches []string
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
			for _, match := range grepResults {
				highlighted := regexp.MustCompile("(?i)(" + pattern + ")").ReplaceAllString(match.Content, "\x1b[31m$1\x1b[0m")
				matches = append(matches, fmt.Sprintf("%s:%d:%s", match.FileName, match.LineNumber, highlighted))
			}

			if len(matches) > 0 {
				results <- grepResult{dir: dir, matches: matches}
			}
		}(entry.Name())
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		for _, match := range result.matches {
			fmt.Printf("\x1b[34m%s\x1b[0m:%s\n", result.dir, match)
		}
	}

	return nil
}