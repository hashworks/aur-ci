package server

import (
	"github.com/hashworks/aur-ci/controller/model"
	"xorm.io/xorm"
)

func (s *Server) searchRunningOrCreatedWorkers() *xorm.Session {
	return s.DB.Table("worker").
		Where("status = ? OR status = ?", model.WORKER_STATUS_RUNNING, model.WORKER_STATUS_CREATED)
}
