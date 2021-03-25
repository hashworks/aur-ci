package server

import (
	"context"
	"log"
	"time"

	"github.com/hashworks/aur-ci/controller/model"
	"github.com/hetznercloud/hcloud-go/hcloud"
)

const MAX_VM_COUNT = 1
const BUILD_QUEUE_TRESHOLD = 0         // 10?
const HETZNER_SERVER_TYPE = "cpx11"    // https://api.hetzner.cloud/v1/server_types
const HETZNER_SERVER_LOCATION = "nbg1" // https://api.hetzner.cloud/v1/locations

// TODO: This way builds that should receive a "timeout" state might get back into the queue. Mark as "timeout" if runtime > threshold?
func (s *Server) destroyExpiredVMs() {
	// get list of vms / workers that are our vms where uptime is > 55m
	// foreach vm: destroy vm, mark related builds as unbuild

	// get list of VMs that haven't reported back at all (WORKER_STATUS_CREATED (not running) and CreatedAt is older than 10m)
	// foreach vm: destroy vm, report to monitoring
}

// TODO: Estimate if we need to increase the MaxVMCount. Did the last VM build less packages than arrived in its existence?
func (s *Server) createRequiredVMs() {
	// get count of running VMs – is it below MaxVMCount?
	vmCount, err := s.DB.Table("worker").Where("status = ?", model.WORKER_STATUS_RUNNING).Count()
	if err != nil {
		log.Println("Failed to count running workers:", err.Error())
		return
	}
	log.Println("DEBUG: vmCount:", vmCount)

	if vmCount >= MAX_VM_COUNT {
		return
	}

	// get list of STATUS_PENDING builds with TYPE_PACKAGE (limit to last 24h)
	var buildQueue []*model.Build
	err = s.DB.Where("status = ? AND type = ? AND created_at > ?", model.STATUS_PENDING, model.TYPE_PACKAGE, time.Now().AddDate(0, 0, -1)).
		Asc("created_at").
		Find(buildQueue)
	if err != nil {
		log.Println("Error: Failed to receive build queue,", err.Error())
		return
	}
	log.Println("DEBUG: len buildQueue:", len(buildQueue))

	// is the count above BuildQueueTreshold? Or is the oldest package by change older than 10h?
	if len(buildQueue) == 0 || len(buildQueue) < BUILD_QUEUE_TRESHOLD || buildQueue[0].CreatedAt.After(time.Now().Add(-10*time.Hour)) {
		log.Println("DEBUG: Not creating a new VM")
		return
	}

	log.Println("DEBUG: Creating a new VM")

	// create vm
	ctx := context.Background() // TODO: Evaluate proper context usage

	createOpts, err := s.getServerCreateOpts("worker-" + time.Now().Format("2006-01-02-15-04-05"))
	if err != nil {
		log.Println("Error: Failed create hetzner server options,", err.Error())
		return
	}
	result, _, err := s.HetznerClient.Server.Create(ctx, createOpts)
	if err != nil {
		log.Println("Error: Failed to create hetzner server,", err.Error())
		return
	}

	log.Println("DEBUG: Created", result.Server.Name)

	_, err = s.DB.Insert(model.NewWorkerFromHetznerServer(result.Server))
	if err != nil {
		log.Printf("Error: Failed to insert new server %s (id %d), %s", result.Server.Name, result.Server.ID, err.Error())

		_, err = s.HetznerClient.Server.Delete(ctx, result.Server)
		if err != nil {
			log.Fatal("Fatal: Failed to delete unsaved hetzner server,", err.Error())
		}

		return
	}
}

func (s *Server) CheckVMStatus() {
	s.destroyExpiredVMs()
	s.createRequiredVMs()
}

func (s *Server) getServerCreateOpts(name string) (hcloud.ServerCreateOpts, error) {
	var err error
	ctx := context.Background()
	createOpts := hcloud.ServerCreateOpts{
		Name:             name,
		StartAfterCreate: hcloud.Bool(true),
	}

	createOpts.ServerType, _, err = s.HetznerClient.ServerType.GetByName(ctx, HETZNER_SERVER_TYPE)
	if err != nil {
		return createOpts, err
	}

	// TODO: Replace with Arch Image
	createOpts.Image, _, err = s.HetznerClient.Image.GetByName(ctx, "debian-10")
	if err != nil {
		return createOpts, err
	}

	if len(*s.HetznerSSHKeyName) > 0 {
		sshKey, _, err := s.HetznerClient.SSHKey.GetByName(ctx, *s.HetznerSSHKeyName)
		if err != nil {
			return createOpts, err
		}
		createOpts.SSHKeys = append(createOpts.SSHKeys, sshKey)
	}

	createOpts.Location, _, err = s.HetznerClient.Location.GetByName(ctx, HETZNER_SERVER_LOCATION)
	if err != nil {
		return createOpts, err
	}

	return createOpts, nil

}
