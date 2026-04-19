package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

var (
	cloneFile          string
	parallelism        int
	cloneBranch        string
	cloneDepth         int
	cloneRecurseSubmodules bool
	cloneSingleBranch  bool
	cloneBare          bool
)

type cloneResult struct {
	url  string
	dir  string
	err  error
}

var cloneCmd = &cobra.Command{
	Use:   "clone",
	Short: "Clone multiple git repos in parallel from a file containing URLs",
	RunE:  runClone,
}

func init() {
	cloneCmd.Flags().StringVarP(&cloneFile, "file", "f", "", "File containing git URLs to clone (one per line)")
	cloneCmd.Flags().IntVarP(&parallelism, "parallelism", "c", 8, "Number of clones to run in parallel")
	cloneCmd.Flags().StringVarP(&cloneBranch, "branch", "b", "", "Branch to checkout after clone")
	cloneCmd.Flags().IntVar(&cloneDepth, "depth", 0, "Create a shallow clone with the specified history depth (0 for full clone)")
	cloneCmd.Flags().BoolVar(&cloneRecurseSubmodules, "recurse-submodules", false, "Initialize and clone submodules")
	cloneCmd.Flags().BoolVar(&cloneSingleBranch, "single-branch", false, "Clone only the specified branch")
	cloneCmd.Flags().BoolVar(&cloneBare, "bare", false, "Clone as a bare repository")
	rootCmd.AddCommand(cloneCmd)
}

func runClone(cmd *cobra.Command, args []string) error {
	if cloneFile == "" {
		return fmt.Errorf("file flag (-f) is required")
	}

	data, err := os.ReadFile(cloneFile)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	urls := parseURLs(string(data))
	if len(urls) == 0 {
		return fmt.Errorf("no URLs found in file")
	}

	if parallelism <= 0 {
		parallelism = 8
	}

	sem := make(chan struct{}, parallelism)
	results := make(chan cloneResult, len(urls))
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dir := filepath.Base(url)
			dir = strings.TrimSuffix(dir, ".git")

			co := &git.CloneOptions{
				URL: url,
			}
			if cloneBranch != "" {
				co.ReferenceName = plumbing.NewBranchReferenceName(cloneBranch)
			}
			if cloneDepth > 0 {
				co.Depth = cloneDepth
			}
			if cloneRecurseSubmodules {
				co.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
			}
			if cloneSingleBranch {
				co.SingleBranch = cloneSingleBranch
			}
			if cloneBare {
				co.Bare = cloneBare
			}

			_, err := git.PlainClone(dir, co)

			results <- cloneResult{url: url, dir: dir, err: err}
		}(url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			fmt.Printf("❌ Failed to clone %s: %v\n", result.url, result.err)
		} else {
			fmt.Printf("✅ Cloned %s to %s\n", result.url, result.dir)
		}
	}

	return nil
}

func parseURLs(data string) []string {
	var urls []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			urls = append(urls, line)
		}
	}
	return urls
}