package aur

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/hashworks/aur-ci/controller/model"
)

func CloneOrFetchRepository(gitStoragePath *string, packageBase string) (*git.Repository, error) {
	repositoryPath := filepath.Join(*gitStoragePath, packageBase+".git")

	_, err := os.Stat(repositoryPath)

	var repository *git.Repository

	if os.IsNotExist(err) {
		repository, err = git.PlainClone(repositoryPath, true, &git.CloneOptions{
			URL:      fmt.Sprintf("https://aur.archlinux.org/%s.git", packageBase),
			Progress: nil,
		})
		if err != nil {
			return repository, err
		}
	} else {
		repository, err = git.PlainOpen(repositoryPath)
		if err != nil {
			return repository, err
		}
		err = repository.Fetch(&git.FetchOptions{
			Progress: nil,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return repository, err
		}
	}

	return repository, nil
}

func GetCommitsUntilHash(repository *git.Repository, packageBaseId int64, hash string) ([]model.Commit, error) {
	var commits []model.Commit
	commitIter, err := repository.Log(&git.LogOptions{})

	if err == nil {
		commitIter.ForEach(func(c *object.Commit) error {
			if c.Hash.String() != hash {
				commits = append(commits, model.NewCommitFromGitCommit(packageBaseId, c))
				return nil
			} else {
				return storer.ErrStop
			}
		})
	}

	return commits, err
}
