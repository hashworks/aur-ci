package model

import "time"

type WorkResultStatus int8

const (
	WORK_RESULT_STATUS_INTERNAL_ERROR WorkResultStatus = 0
	WORK_RESULT_STATUS_TIMEOUT        WorkResultStatus = 10
	WORK_RESULT_STATUS_FAILED         WorkResultStatus = 20
	WORK_RESULT_STATUS_SUCCESS        WorkResultStatus = 30
)

type WorkResult struct {
	Id                      int64
	BuildId                 int64
	Status                  WorkResultStatus
	PacmanExitCode          int
	PacmanLogBase64         string
	MakepkgExtractExitCode  int
	MakepkgExtractLogBase64 string
	MakepkgBuildExitCode    int
	MakepkgBuildLogBase64   string
	CreatedAt               time.Time `xorm:"created"`
}

func (r *WorkResult) GetBuildStatus() BuildStatus {
	switch r.Status {
	case WORK_RESULT_STATUS_TIMEOUT:
		return STATUS_TIMEOUT
	case WORK_RESULT_STATUS_FAILED:
		return STATUS_FAILED
	case WORK_RESULT_STATUS_SUCCESS:
		return STATUS_BUILD
	default:
		return STATUS_PENDING
	}
}
