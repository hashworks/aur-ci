package aur

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/hashworks/aur-ci/controller/model"
)

func getRepositoryPath(gitStoragePath *string, packageBase string) string {
	return filepath.Join(*gitStoragePath, packageBase+".git")
}

func GetCommitTAR(gitStoragePath *string, packageBase string, commitHash string) ([]byte, error) {
	if packageBase == "hiawatha-monitor" {
		// https://github.com/go-git/go-git/issues/302
		return []byte{}, errors.New("Ignoring hiawatha-monitor due to issues with go-git")
	}

	repositoryPath := getRepositoryPath(gitStoragePath, packageBase)

	memoryRepository, err := git.Clone(memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL: repositoryPath,
	})

	worktree, err := memoryRepository.Worktree()
	if err != nil {
		return []byte{}, err
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(commitHash),
	})
	if err != nil {
		return []byte{}, err
	}

	files, err := worktree.Filesystem.ReadDir("/")
	if err != nil {
		return []byte{}, err
	}

	var tarBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuffer)

	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue
		}

		name := filepath.Join(packageBase, fileInfo.Name())

		if fileInfo.Mode()&fs.ModeSymlink != 0 {
			linkName, err := worktree.Filesystem.Readlink(fileInfo.Name())
			if err != nil {
				return []byte{}, err
			}
			header, err := tar.FileInfoHeader(fileInfo, linkName)
			if err != nil {
				return []byte{}, err
			}
			header.Name = name
			if err := tarWriter.WriteHeader(header); err != nil {
				return []byte{}, err
			}
		} else {
			file, err := worktree.Filesystem.Open(fileInfo.Name())
			if err != nil {
				return []byte{}, err
			}
			data := make([]byte, fileInfo.Size())
			_, err = file.Read(data)
			if err != nil && err != io.EOF {
				return []byte{}, err
			}

			header, err := tar.FileInfoHeader(fileInfo, "")
			if err != nil {
				return []byte{}, err
			}
			header.Name = name

			if err := tarWriter.WriteHeader(header); err != nil {
				return []byte{}, err
			}
			if _, err := tarWriter.Write(data); err != nil {
				return []byte{}, err
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		return []byte{}, err
	}

	return tarBuffer.Bytes(), nil
}

func CloneOrFetchRepository(gitStoragePath *string, packageBase string) (*git.Repository, error) {
	repositoryPath := getRepositoryPath(gitStoragePath, packageBase)

	_, err := os.Stat(repositoryPath)

	var repository *git.Repository

	if os.IsNotExist(err) {
		repository, err = git.PlainClone(repositoryPath, true, &git.CloneOptions{
			URL:        fmt.Sprintf("https://aur.archlinux.org/%s.git", packageBase),
			Progress:   nil,
			RemoteName: "origin",
		})
		if err != nil {
			return repository, err
		}
	} else {
		repository, err = git.PlainOpen(repositoryPath)
		if err != nil {
			return repository, err
		}
		// git fetch origin +master:master
		err = repository.Fetch(&git.FetchOptions{
			Progress:   nil,
			RemoteName: "origin",
			RefSpecs:   []config.RefSpec{"+refs/heads/master:refs/heads/master"},
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
