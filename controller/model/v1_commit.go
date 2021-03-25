package model

import (
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
)

type Commit struct {
	Id             int64
	PackageBaseId  int64  `xorm:"notnull"`
	Hash           string `xorm:"notnull"`
	Message        string
	AuthorName     string
	AuthorEmail    string
	AuthorWhen     time.Time
	CommitterName  string
	CommitterEmail string
	CommitterWhen  time.Time `xorm:"notnull"`
	ParentHashes   []string
}

func NewCommitFromGitCommit(packageBaseId int64, commit *object.Commit) Commit {
	parentHashes := make([]string, len(commit.ParentHashes))
	for i, parentHash := range commit.ParentHashes {
		parentHashes[i] = parentHash.String()
	}

	return Commit{
		PackageBaseId:  packageBaseId,
		Hash:           commit.Hash.String(),
		Message:        commit.Message,
		AuthorName:     commit.Author.Name,
		AuthorEmail:    commit.Author.Email,
		AuthorWhen:     commit.Author.When,
		CommitterName:  commit.Committer.Name,
		CommitterEmail: commit.Committer.Email,
		CommitterWhen:  commit.Committer.When,
		ParentHashes:   parentHashes,
	}
}
