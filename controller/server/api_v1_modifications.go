package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage"
	"github.com/hashworks/aur-ci/controller/aur"
	"github.com/hashworks/aur-ci/controller/model"

	"github.com/gin-gonic/gin"
)

// @Summary Report packages as modified
// @Success 204
// @Failure 400
// @Accept json
// @Param names body []string true "List of package names, max 250"
// @Router /v1/reportPackageModification [post]
// @Tags V1
func (s *Server) apiV1ReportPackageModification(c *gin.Context) {
	var packageNames []string
	err := c.ShouldBindJSON(&packageNames)

	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	// In theory the RPC endpoint can handle more than 250 packages at once (5000 by default),
	// but we don't really need it and can limit abuse a bit.
	if len(packageNames) == 0 || len(packageNames) > 250 {
		c.AbortWithError(http.StatusBadRequest, errors.New("Packagenames outside of range {1,250}"))
		return
	}

	pkgs, err := aur.GetPackageInfos(packageNames)
	if err != nil {
		c.Error(errors.New(fmt.Sprintf("Failed to receive package infos for %d packages", len(packageNames))))
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	packageBases := make(map[string]int64)
	for _, pkg := range pkgs {
		updateCount, err := s.DB.Update(pkg, model.Package{Name: pkg.Name})
		if err != nil {
			c.Error(errors.New(fmt.Sprintf("Failed to update package %s in database", pkg.Name)))
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		if updateCount == 0 {
			_, err = s.DB.Insert(pkg)
			if err != nil {
				c.Error(errors.New(fmt.Sprintf("Failed to insert package %s into database", pkg.Name)))
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
		}
		packageBases[pkg.PackageBase] = pkg.PackageBaseId
	}

PACKAGES:
	for packageBase, packageBaseId := range packageBases {
		var repository *git.Repository

	TRIES:
		for try := 0; ; try++ {
			repository, err = aur.CloneOrFetchRepository(s.GitStoragePath, packageBase)
			if err != nil {
				if err == storage.ErrReferenceHasChanged {
					if try == 2 {
						c.Error(errors.New(fmt.Sprintf("Failed to clone or fetch package base %s", packageBase)))
						c.AbortWithError(http.StatusInternalServerError, err)
						return
					}
					// On some filesystems (f.e. xfs) this error can occour. No solution yet, but only occours sporadicaly.
					// see https://github.com/go-git/go-git/issues/37
					log.Printf("Warning: ErrReferenceHasChanged. Retrying package base %s.", packageBase)
					continue TRIES
				} else if err.Error() == "unexpected client error: "+plumbing.ErrReferenceNotFound.Error() {
					// Some (read: three) AUR repos do not have a branch (detached HEAD). No idea how to handle this,
					// see https://github.com/go-git/go-git/issues/270
					// Let's ignore them until then. However, we should fix them!
					log.Printf("Warning: Skipping package base %s since it has no branch set.", packageBase)
					continue PACKAGES
				}
				c.Error(errors.New(fmt.Sprintf("Failed to clone or fetch package base %s", packageBase)))
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
			break TRIES
		}

		var lastHash string
		_, err = s.getLastCommitOfPackageBaseId(packageBaseId).Cols("hash").Get(&lastHash)
		if err != nil {
			c.Error(errors.New(fmt.Sprintf("Failed to select last hash of package base %s (id %d)", packageBase, packageBaseId)))
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		newCommits, err := aur.GetCommitsUntilHash(repository, packageBaseId, lastHash)
		if err != nil {
			c.Error(errors.New(fmt.Sprintf("Failed to get commits of package base %s (id %d)", packageBase, packageBaseId)))
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		if len(newCommits) > 0 {
			_, err = s.DB.Insert(newCommits)
			if err != nil {
				c.Error(errors.New(fmt.Sprintf("Failed to insert new commits of package base %s (id %d)", packageBase, packageBaseId)))
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}

			// TODO: Depends list! This can be recursive, move to function
			// Get list of all depends and make_depends (by getting all packages of a packageBaseId)
			// Filter depends/make_depends by AUR packages
			// Get last commit for every depends/make_depends
			// Add build task for last_commit if it doesn't exist already
			// Save list of build task ids

			var lastCommitId int64
			_, err = s.getLastCommitOfPackageBaseId(packageBaseId).Cols("id").Get(&lastCommitId)
			if err != nil {
				c.Error(errors.New(fmt.Sprintf("Failed to select last commit id of package base %s (id %d)", packageBase, packageBaseId)))
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}

			_, err = s.DB.Insert(model.Build{
				PackageBase:   packageBase,
				PackageBaseId: packageBaseId,
				CommitId:      lastCommitId,
				Status:        model.STATUS_PENDING,
				Type:          model.TYPE_PACKAGE,
			})
			if err != nil {
				c.Error(errors.New(fmt.Sprintf("Failed to insert new build task of package base %s (id %d)", packageBase, packageBaseId)))
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
		}
	}

	c.Status(http.StatusNoContent)
}
