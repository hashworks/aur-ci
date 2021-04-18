package model

// TODO: Use controller model files

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
}
