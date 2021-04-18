package model

import (
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud"
)

type WorkerType int8
type WorkerStatus int8

const (
	WORKER_TYPE_OTHER   WorkerType = 0
	WORKER_TYPE_HETZNER WorkerType = 10
)

const (
	WORKER_STATUS_CREATED WorkerStatus = 10
	WORKER_STATUS_RUNNING WorkerStatus = 20
	WORKER_STATUS_STOPPED WorkerStatus = 30
)

type Worker struct {
	Id        int64
	Type      WorkerType
	Status    WorkerStatus
	HetznerId int
	Name      string
	IPv4      string    `xorm:"'ipv4'"`
	IPv6      string    `xorm:"'ipv6'"`
	CreatedAt time.Time `xorm:"created"`
	UpdatedAt time.Time `xorm:"updated"`
}

func NewWorkerFromHetznerServer(server *hcloud.Server) Worker {
	return Worker{
		Type:      WORKER_TYPE_HETZNER,
		Status:    WORKER_STATUS_CREATED,
		HetznerId: server.ID,
		Name:      server.Name,
		IPv4:      server.PublicNet.IPv4.IP.String(),
		IPv6:      server.PublicNet.IPv6.IP.String(),
		CreatedAt: server.Created,
	}
}
