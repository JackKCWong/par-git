package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-git/v6"
	"github.com/spf13/cobra"
)

var (
	pullDirectory      string
	pullParallelism    int
	pullForce          bool
	pullDepth          int
	pullRecurseSubmodules bool
)

type pullResult struct {
	dir string
	err error
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull all git repos in cwd in parallel",
	RunE:  runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullDirectory, "directory", "C", "", "Root directory to search for git repos (default cwd)")
	pullCmd.Flags().IntVarP(&pullParallelism, "parallelism", "c", 8, "Number of pulls to run in parallel")
	pullCmd.Flags().BoolVarP(&pullForce, "force", "f", false, "Force pull (discard local changes)")
	pullCmd.Flags().IntVarP(&pullDepth, "depth", "d", 0, "Depth for shallow clones")
	pullCmd.Flags().BoolVarP(&pullRecurseSubmodules, "recurse-submodules", "", false, "Recursively pull submodules")
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	if pullDirectory == "" {
		pullDirectory = "."
	}

	entries, err := os.ReadDir(pullDirectory)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var repos []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		gitPath := filepath.Join(pullDirectory, entry.Name(), ".git")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue
		}
		repos = append(repos, entry.Name())
	}

	if len(repos) == 0 {
		return fmt.Errorf("no git repos found in %s", pullDirectory)
	}

	if pullParallelism <= 0 {
		pullParallelism = 8
	}

	sem := make(chan struct{}, pullParallelism)
	results := make(chan pullResult, len(repos))
	var wg sync.WaitGroup

	for _, repo := range repos {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			repoPath := filepath.Join(pullDirectory, dir)
			r, err := git.PlainOpen(repoPath)
			if err != nil {
				results <- pullResult{dir: dir, err: fmt.Errorf("failed to open repo: %w", err)}
				return
			}

			worktree, err := r.Worktree()
			if err != nil {
				results <- pullResult{dir: dir, err: fmt.Errorf("failed to get worktree: %w", err)}
				return
			}

			pullOpts := &git.PullOptions{}
			if pullForce {
				pullOpts.Force = pullForce
			}
			if pullDepth > 0 {
				pullOpts.Depth = pullDepth
			}
			if pullRecurseSubmodules {
				pullOpts.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
			}
			if err := worktree.Pull(pullOpts); err != nil && err != git.NoErrAlreadyUpToDate {
				results <- pullResult{dir: dir, err: err}
				return
			}

			results <- pullResult{dir: dir, err: nil}
		}(repo)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var errors []pullResult
	for result := range results {
		if result.err != nil {
			errors = append(errors, result)
		} else {
			fmt.Printf("✅ Pulled %s\n", result.dir)
		}
	}

	if len(errors) > 0 {
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "❌ Failed to pull %s: %v\n", e.dir, e.err)
		}
		return fmt.Errorf("%d repo(s) failed to pull", len(errors))
	}

	return nil
}