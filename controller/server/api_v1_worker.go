package server

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hashworks/aur-ci/controller/aur"
	"github.com/hashworks/aur-ci/controller/model"
)

// @Summary Endpoint for workers to report work results.
// @Success 204
// @Failure 400
// @Failure 404 Worker or build not found, send heartbeat or request first
// @Accept json
// @Param result body model.WorkResult true "The result of the work"
// @Router /v1/worker/reportWorkResult [put]
// @Tags V1
func (s *Server) apiV1WorkerReportWorkResult(c *gin.Context) {
	var workResult model.WorkResult
	if err := c.BindJSON(&workResult); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// TODO: We should handle IPv6 addresses as well
	// TODO: Switch to per-worker API-Keys to identify them
	worker := model.Worker{
		IPv4: c.ClientIP(),
	}
	workerExists, err := s.DB.Get(&worker)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get worker from database: "+err.Error()))
		return
	}
	if !workerExists {
		c.AbortWithError(http.StatusNotFound, errors.New("Worker not found"))
		return
	}

	build := model.Build{
		Id: workResult.BuildId,
	}
	buildExists, err := s.DB.Get(&build)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get build from database: "+err.Error()))
		return
	}
	if !buildExists {
		c.AbortWithError(http.StatusNotFound, errors.New("Build not found"))
		return
	}

	if _, err := s.DB.Insert(workResult); err != nil {
		// TODO: Rollback?
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to insert work result into database: "+err.Error()))
		return
	}

	build.Status = workResult.GetBuildStatus()
	if build.Status != model.STATUS_PENDING {
		build.FinishedAt = time.Now()
	}

	_, err = s.DB.Update(build, model.Build{Id: workResult.BuildId})

	if err != nil {
		// TODO: Rollback?
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to update build in database: "+err.Error()))
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Endpoint for workers to request work.
// @Produce json
// @Success 200 {array} model.Work
// @Failure 400
// @Failure 404 Worker not found, send heartbeat first
// @Param amount query int false "Work amount to request, default 1"
// @Router /v1/worker/requestWork [get]
// @Tags V1
func (s *Server) apiV1WorkerRequestWork(c *gin.Context) {
	// TODO: We should handle IPv6 addresses as well
	// TODO: Switch to per-worker API-Keys to identify them
	worker := model.Worker{
		IPv4: c.ClientIP(),
	}
	workerExists, err := s.DB.Get(&worker)

	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get worker from database: "+err.Error()))
		return
	}
	if !workerExists {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	// TODO: Handle multiple workers requesting work at the same time.
	// Multithread lock? Wait or tell them to try again?

	amount, err := strconv.ParseInt(c.DefaultQuery("amount", "1"), 10, 32)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if amount < 1 {
		amount = 1
	}

	var pendingBuilds []model.Build
	if err := s.searchPendingPackageBuilds().Limit(int(amount)).Find(&pendingBuilds); err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get build from database: "+err.Error()))
		return
	}

	workList := make([]model.Work, 0)

	for i := range pendingBuilds {

		pendingBuilds[i].WorkerId = worker.Id
		pendingBuilds[i].Status = model.STATUS_BUILDING
		pendingBuilds[i].StartedAt = time.Now()

		var dependencies []string

		var pkg model.Package
		if _, err := s.DB.Table("package").
			Cols("depends", "make_depends", "check_depends").
			Where("package_base_id = ?", pendingBuilds[i].PackageBaseId).
			Get(&pkg); err != nil {
			// TODO: Rollback?
			c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get dependencies: "+err.Error()))
			return
		}
		for _, dependency := range pkg.Depends {
			dependencies = append(dependencies, dependency)
		}
		for _, dependency := range pkg.MakeDepends {
			dependencies = append(dependencies, dependency)
		}
		for _, dependency := range pkg.CheckDepends {
			dependencies = append(dependencies, dependency)
		}

		// TODO: Get all dependency builds and priorise them – this can be recursive!

		var commitHash string
		if _, err := s.DB.Table("commit").
			Cols("hash").
			Where("id = ?", pendingBuilds[i].CommitId).
			Get(&commitHash); err != nil {
			// TODO: Rollback?
			c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get commit: "+err.Error()))
			return
		}

		tarBytes, err := aur.GetCommitTAR(s.GitStoragePath, pendingBuilds[i].PackageBase, commitHash)
		if err != nil {
			// TODO: Rollback?
			c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get commit tar: "+err.Error()))
			return
		}

		if _, err := s.DB.Update(&pendingBuilds[i], model.Build{Id: pendingBuilds[i].Id}); err != nil {
			// TODO: Rollback?
			c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to update build in database: "+err.Error()))
			return
		}

		workList = append(workList, model.Work{
			BuildId:               pendingBuilds[i].Id,
			PackageBase:           pendingBuilds[i].PackageBase,
			Dependencies:          dependencies,
			PackageBaseDataBase64: base64.StdEncoding.EncodeToString(tarBytes),
		})
	}

	c.JSON(http.StatusOK, workList)
}

// @Summary Receives a heartbeat from a worker. Can also be used to register a new worker.
// @Success 204
// @Failure 400
// @Param hostname path string true "Hostname"
// @Router /v1/worker/heartbeat/{hostname} [post]
// @Tags V1
func (s *Server) apiV1WorkerHeartbeat(c *gin.Context) {
	hostname := c.Param("hostname")
	if len(hostname) == 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	ip := c.ClientIP()

	// TODO: We should handle IPv6 addresses as well
	// TODO: Switch to per-worker API-Keys to identify them
	worker := model.Worker{
		IPv4: ip,
	}
	workerExists, err := s.DB.Get(&worker)

	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to get worker from database: "+err.Error()))
		return
	}

	if !workerExists {
		worker := model.Worker{
			Name:   hostname,
			IPv4:   ip,
			Status: model.WORKER_STATUS_RUNNING,
			Type:   model.WORKER_TYPE_OTHER,
		}
		if _, err := s.DB.Insert(&worker); err != nil {
			c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to insert worker into database: "+err.Error()))
			return
		}
	} else {
		worker.Status = model.WORKER_STATUS_RUNNING
		if _, err := s.DB.Update(&worker, model.Worker{Id: worker.Id}); err != nil {
			c.AbortWithError(http.StatusInternalServerError, errors.New("Failed to update worker in database: "+err.Error()))
			return
		}
	}

	c.Status(http.StatusNoContent)
}
