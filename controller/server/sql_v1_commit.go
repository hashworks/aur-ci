package server

import "xorm.io/xorm"

func (s *Server) getLastCommitOfPackageBaseId(packageBaseId int64) *xorm.Session {
	return s.DB.Table("commit").Where("package_base_id = ?", packageBaseId).Desc("committer_when")
}
