package server

import (
	"time"

	"github.com/hashworks/aur-ci/controller/model"
	"xorm.io/xorm"
)

func (s *Server) searchPendingPackageBuilds() *xorm.Session {
	// Limited to the last 24h
	return s.DB.Table("build").Where("status = ? AND type = ? AND created_at > ?", model.STATUS_PENDING, model.TYPE_PACKAGE, time.Now().AddDate(0, 0, -1)).
		Asc("created_at")
}
