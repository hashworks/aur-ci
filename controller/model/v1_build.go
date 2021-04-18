package model

import "time"

type BuildStatus int8
type BuildType int8

const (
	STATUS_PENDING  BuildStatus = 10
	STATUS_BUILDING BuildStatus = 20
	STATUS_TIMEOUT  BuildStatus = 30
	STATUS_FAILED   BuildStatus = 40
	STATUS_BUILD    BuildStatus = 50
)

const (
	TYPE_PACKAGE    BuildType = 10
	TYPE_DEPENDENCY BuildType = 20
)

type Build struct {
	Id                int64
	PackageBase       string
	PackageBaseId     int64
	CommitId          int64
	WorkerId          int64
	Status            BuildStatus
	Type              BuildType
	DependsOnBuildIds []int64
	CreatedAt         time.Time `xorm:"created"`
	StartedAt         time.Time
	FinishedAt        time.Time
}
