package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashworks/aur-ci/controller/model"
	"github.com/hetznercloud/hcloud-go/hcloud"
)

const MAX_VM_COUNT = 1
const BUILD_QUEUE_TRESHOLD = 10
const HETZNER_SERVER_TYPE = "cpx11"    // https://api.hetzner.cloud/v1/server_types
const HETZNER_SERVER_LOCATION = "nbg1" // https://api.hetzner.cloud/v1/locations

func (s *Server) markBuildsOfWorkerAsPending(worker *model.Worker) {
	_, err := s.DB.
		Where("worker_id = ? AND status = ?",
			worker.Id,
			model.STATUS_BUILDING).
		Update(model.Build{
			Status:   model.STATUS_PENDING,
			WorkerId: 0,
		})
	if err != nil {
		log.Println("Error: Failed to update builds of worker:", err)
	}
}

// Remove Hetzner VMs that are older than 55 minutes
// Also marks their builds as pending, putting them back into the queue.
// TODO: This way builds that should receive a "timeout" state might get back into the queue. Mark as "timeout" if runtime > threshold?
func (s *Server) removeExpiredVMs() {
	ctx := context.Background()

	var expiredHetznerVMs []model.Worker
	err := s.DB.
		Where("type == ? AND status != ? AND DATETIME(created_at) < ?",
			model.WORKER_TYPE_HETZNER,
			model.WORKER_STATUS_STOPPED,
			time.Now().UTC().Add(-55*time.Minute)).
		Find(&expiredHetznerVMs)
	if err != nil {
		log.Println("Error: Failed to find expired hetzner VMs:", err)
	}

	for _, expiredHetznerVM := range expiredHetznerVMs {
		log.Println("Removing hetzner VM", expiredHetznerVM.Name)

		server, _, err := s.HetznerClient.Server.GetByID(ctx, expiredHetznerVM.HetznerId)
		if err != nil {
			log.Println("Error: Failed to get expired hetzner VM by id:", err)
			continue
		}

		if server != nil {
			_, err = s.HetznerClient.Server.Delete(ctx, server)
			if err != nil {
				log.Println("Error: Failed to delete expired hetzner VM:", err)
				continue
			}
		} else {
			log.Printf("Warning: Failed to remove hetzner VM with id %d. Maybe it was removed already.\n", expiredHetznerVM.HetznerId)
		}

		_, err = s.DB.Update(model.Worker{
			Status: model.WORKER_STATUS_STOPPED,
		}, model.Worker{
			Id: expiredHetznerVM.Id,
		})
		if err != nil {
			log.Println("Error: Failed to update status of expired hetzner VM in database:", err)
			continue
		}

		s.markBuildsOfWorkerAsPending(&expiredHetznerVM)
	}
}

// Mark workers that haven't reported back since some while (no heartbeat) as stopped
// Also marks their builds as pending, putting them back into the queue.
func (s *Server) markTimedOutWorkersAsStopped() {
	var timedOutWorkers []model.Worker
	err := s.DB.
		Where("status != ? AND DATETIME(updated_at) < ?",
			model.WORKER_STATUS_STOPPED,
			time.Now().UTC().Add(-10*time.Minute)).
		Find(&timedOutWorkers)
	if err != nil {
		log.Println("Error: Failed to find timed out workers: ", err)
	}
	for _, timedOutWorker := range timedOutWorkers {
		s.markBuildsOfWorkerAsPending(&timedOutWorker)
	}
}

// TODO: Estimate if we need to increase the MaxVMCount. Did the last VM build less packages than arrived in its existence?
func (s *Server) createRequiredVMs() {
	// get count of running VMs – is it below MaxVMCount?
	vmCount, err := s.searchRunningOrCreatedWorkers().Count()
	if err != nil {
		log.Println("Error: Failed to count workers:", err.Error())
		return
	}

	if vmCount >= MAX_VM_COUNT {
		return
	}

	var buildQueue []model.Build
	err = s.searchPendingPackageBuilds().Find(&buildQueue)
	if err != nil {
		log.Println("Error: Failed to receive build queue,", err.Error())
		return
	}

	// is the count >= BuildQueueTreshold? Or is the oldest package by change older than 10h?
	if len(buildQueue) == 0 || len(buildQueue) < BUILD_QUEUE_TRESHOLD || buildQueue[0].CreatedAt.Before(time.Now().Add(-10*time.Hour)) {
		return
	}

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

	log.Println("Created hetzner VM", result.Server.Name)

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
	s.removeExpiredVMs()
	s.markTimedOutWorkersAsStopped()
	s.createRequiredVMs()
}

func (s *Server) getServerCreateOpts(name string) (hcloud.ServerCreateOpts, error) {
	var err error
	ctx := context.Background()
	createOpts := hcloud.ServerCreateOpts{
		Name:             name,
		StartAfterCreate: hcloud.Bool(true),
		UserData: fmt.Sprintf(`#cloud-config
write_files:
- content: |
    [Unit]
    Description=AUR CI Worker
    After=docker.service
    After=network-online.target
    Wants=network-online.target

    [Service]
    ExecStart=/usr/local/bin/aur-ci-worker -controller '%s' -work-amount 1
    DynamicUser=yes
    ProtectSystem=strict
    PrivateTmp=yes
    NoNewPrivileges=yes
    ProtectControlGroups=yes
    ProtectKernelTunables=yes
    RemoveIPC=yes
    Group=docker
    Restart=always

    [Install]
    WantedBy=default.target
  path: /etc/systemd/system/aur-ci-worker.service
runcmd:
- dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
- dnf install -y docker-ce
- curl -s -o /usr/local/bin/aur-ci-worker '%s'
- chmod +x /usr/local/bin/aur-ci-worker
- systemctl enable --now docker aur-ci-worker.service`, *s.ExternalURI, "https://fb.hash.works/bAKdSn3/"),
	}
	// TODO: Provide aur-ci-worker binary with controller endpoint

	createOpts.ServerType, _, err = s.HetznerClient.ServerType.GetByName(ctx, HETZNER_SERVER_TYPE)
	if err != nil {
		return createOpts, err
	}

	// TODO: Replace with Arch Image / Snapshot
	createOpts.Image, _, err = s.HetznerClient.Image.GetByName(ctx, "fedora-33")
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
