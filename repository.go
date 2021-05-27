package main

import (
	"os/exec"
	"sync"
)

type repository interface {
	addCommitAndPush(commitMsg string, resourcePaths []string) (bool, error)
}

type dumpRepository struct {
	worktreeLock sync.Mutex
}

func newRepository(repositoryURL string) (repository, error) {
	if err := cloneRepository(repositoryURL); err != nil {
		return nil, err
	}

	if err := includeLocalConfig(); err != nil {
		return nil, err
	}

	dumpRepository := &dumpRepository{
		worktreeLock: sync.Mutex{},
	}
	return dumpRepository, nil
}

func cloneRepository(repositoryURL string) error {
	return exec.Command("git", "clone", repositoryURL, ".").Run()
}

func includeLocalConfig() error {
	return exec.Command("git", "config", "--local", "include.path", "../.gitconfig").Run()
}

func (r *dumpRepository) addCommitAndPush(commitMsg string, paths []string) (bool, error) {
	r.worktreeLock.Lock()
	defer r.worktreeLock.Unlock()

	if err := r.add(paths); err != nil {
		return false, err
	}

	if checkoutPaths, err := r.checkoutUnchangedFiles(paths); err != nil || len(paths) == len(checkoutPaths) {
		return false, err
	}

	if err := r.commit(commitMsg); err != nil {
		return false, err
	}

	return true, r.push()
}

func (r *dumpRepository) checkoutUnchangedFiles(paths []string) ([]string, error) {
	checkoutPaths := []string{}
	for _, path := range paths {
		diff, err := r.diff(path)
		if err != nil {
			return nil, err
		}

		if !diff {
			checkoutPaths = append(checkoutPaths, path)
		}
	}
	return checkoutPaths, r.checkoutHead(checkoutPaths)
}

func (r *dumpRepository) checkoutHead(paths []string) error {
	args := []string{"checkout", "HEAD"}
	args = append(args, paths...)
	return exec.Command("git", args...).Run()
}

func (r *dumpRepository) add(paths []string) error {
	args := []string{"add"}
	args = append(args, paths...)
	return exec.Command("git", args...).Run()
}

func (r *dumpRepository) diff(path string) (bool, error) {
	out, err := exec.Command("git", "diff", "--cached", "--", path).Output()
	if err != nil {
		return false, err
	}
	return string(out) != "", nil
}

func (r *dumpRepository) commit(msg string) error {
	return exec.Command("git", "commit", "-m", msg).Run()
}

func (r *dumpRepository) push() error {
	return exec.Command("git", "push").Run()
}
