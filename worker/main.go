package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/versions"
	"github.com/hashworks/aur-ci/worker/model"
	"github.com/moby/moby/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/robfig/cron/v3"
)

//go:embed rootfs/home/ci/.makepkg.conf
var makepkgConf []byte
var rootfsTAR []byte

var docker_client *client.Client

var controller_uri *string
var work_amount *int

const DOCKER_CONTAINER_PREFIX = "aur-ci-worker-build"

func sendHeartbeat() error {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal("Failed to get hostname: ", err)
		return err
	}
	response, err := http.Post(*controller_uri+"/api/v1/worker/heartbeat/"+hostname, "", nil)
	if err != nil {
		log.Println("Failed to send heartbeat to controller: ", err)
		return err
	} else if response.StatusCode != http.StatusNoContent {
		log.Printf("Failed to send heartbeat to controller (code %d)\n", response.StatusCode)
		return err
	}
	return nil
}

func requestWork() ([]model.Work, error) {
	var availableWorks []model.Work

	response, err := http.Get(fmt.Sprintf(*controller_uri+"/api/v1/worker/requestWork?amount=%d", *work_amount))
	if err != nil {
		return availableWorks, err
	} else if response.StatusCode != 200 {
		return availableWorks, errors.New(fmt.Sprintf("Unexpected status code %d", response.StatusCode))
	}
	defer response.Body.Close()

	err = json.NewDecoder(response.Body).Decode(&availableWorks)
	return availableWorks, err
}

func sendWorkResult(workResult *model.WorkResult, packageBase string) {
	data, err := json.Marshal(workResult)
	if err != nil {
		log.Printf("[%s] Failed to marshal work result: %s", packageBase, err)
	}

	client := &http.Client{}
	request, err := http.NewRequest("PUT", *controller_uri+"/api/v1/worker/reportWorkResult", bytes.NewReader(data))
	if err != nil {
		log.Printf("[%s] Failed to create work result request: %s", packageBase, err)
		return
	}
	response, err := client.Do(request)
	if err != nil {
		log.Printf("[%s] Failed to report work result: %s", packageBase, err)
	} else if response.StatusCode != http.StatusNoContent {
		log.Printf("[%s] Failed to report work result (code %d)\n", packageBase, response.StatusCode)
	}
}

func getEnv(key string, defaultValue string) string {
	v := os.Getenv(key)
	if len(v) == 0 {
		return defaultValue
	}
	return v
}

func initRootFSTARBuffer() {
	var rootfsTARBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&rootfsTARBuffer)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "home/ci/.makepkg.conf",
		Mode: 0644,
		Size: int64(len(makepkgConf)),
	}); err != nil {
		log.Fatal("Failed to create tar buffer: ", err)
	}
	if _, err := tarWriter.Write(makepkgConf); err != nil {
		log.Fatal("Failed to create tar buffer: ", err)
	}
	if err := tarWriter.Close(); err != nil {
		log.Fatal("Failed to create tar buffer: ", err)
	}
	rootfsTAR = rootfsTARBuffer.Bytes()
}

func initDockerClient() {
	var err error
	docker_client, err = client.NewClientWithOpts(client.FromEnv)
	if err != nil { // Note: This won't be null if the socket is not available
		log.Fatal("Failed to create docker_client: ", err)
	}

	// Test the socket
	if _, err = docker_client.Info(context.Background()); err != nil {
		log.Fatal("Failed to connect to docker daemon: ", err)
	}
}

func pullArchLinuxImage() error {
	log.Println("Pulling Arch Linux image")
	readCloser, err := docker_client.ImagePull(context.Background(), "archlinux/archlinux:base-devel", types.ImagePullOptions{})
	if err != nil {
		log.Println("Failed to pull image: ", err)
		return err
	}
	loadResponse, err := docker_client.ImageLoad(context.Background(), readCloser, true)
	if err != nil {
		log.Println("Failed to load image: ", err)
		return err
	}
	if err := loadResponse.Body.Close(); err != nil {
		log.Println("Failed to close image load response: ", err)
		return err
	}
	if err := readCloser.Close(); err != nil {
		log.Println("Failed to close image reader: ", err)
		return err
	}
	return nil
}

func removeOldContainers() {
	log.Println("Removing old containers")

	ctx := context.Background()

	containers, err := docker_client.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		log.Println("Failed to get list of containers: ", err)
		return
	}
	for _, container := range containers {
		for _, name := range container.Names {
			if strings.HasPrefix(name, "/"+DOCKER_CONTAINER_PREFIX) {
				if err := docker_client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{
					RemoveVolumes: true,
					Force:         true,
				}); err != nil {
					log.Println("Failed to remove old container: ", err)
				}
				break
			}
		}
	}
}

func createContainer(ctx context.Context, work *model.Work) (container.ContainerCreateCreatedBody, error) {
	log.Printf("[%s] Creating container\n", work.PackageBase)

	var platform *v1.Platform
	if versions.GreaterThanOrEqualTo(docker_client.ClientVersion(), "1.41") {
		platform = &v1.Platform{
			Architecture: "amd64",
			OS:           "linux",
		}
	} else {
		platform = nil
	}

	// TODO: We need a long-running command (read: forever) to execute more than one command (exec).
	// This sounds like a hacky way to do things, maybe we should switch to containerd alltogether?
	buildContainer, err := docker_client.ContainerCreate(ctx,
		&container.Config{
			Image: "archlinux/archlinux:base-devel",
			Cmd:   []string{"tail", "-f", "/dev/null"},
		},
		&container.HostConfig{},
		&network.NetworkingConfig{},
		platform,
		fmt.Sprintf("%s-%d-%s", DOCKER_CONTAINER_PREFIX, work.BuildId, work.PackageBase))
	if err != nil {
		log.Printf("[%s] Failed to create container: %s\n", work.PackageBase, err)
		return container.ContainerCreateCreatedBody{}, err
	}

	return buildContainer, nil
}

func runAndWaitForExec(ctx context.Context, cli *client.Client, containerID string, execConfig types.ExecConfig) ([]byte, int, error) {
	idResponse, err := cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, 0, err
	}

	attach, err := cli.ContainerExecAttach(ctx, idResponse.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, 0, err
	}
	defer attach.Close()

	var inspect types.ContainerExecInspect

	for {
		inspect, err = cli.ContainerExecInspect(ctx, idResponse.ID)
		if err != nil {
			return nil, 0, err
		}

		if !inspect.Running {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	scanner := bufio.NewScanner(attach.Reader)
	scanner.Split(bufio.ScanLines)
	var execLog []string

	for scanner.Scan() {
		execLog = append(execLog, scanner.Text())
	}

	return []byte(strings.Join(execLog, "\n")), inspect.ExitCode, nil
}

func prepareContainer(ctx context.Context, work *model.Work, containerId string) error {
	log.Printf("[%s] Preparing container and inserting data\n", work.PackageBase)

	// add user
	_, exitCode, err := runAndWaitForExec(ctx, docker_client, containerId, types.ExecConfig{
		Cmd: []string{
			"bash", "-c", "useradd -m ci; mkdir -p /home/ci/aur",
		},
		AttachStdout: true,
	})
	if err != nil {
		return err
	}
	if exitCode > 0 {
		return errors.New(fmt.Sprintf("Unexpected exit code %d", exitCode))
	}

	// copy rootfs
	if err := docker_client.CopyToContainer(ctx, containerId, "/", bytes.NewReader(rootfsTAR), types.CopyToContainerOptions{}); err != nil {
		return err
	}

	// copy package
	data := make([]byte, base64.StdEncoding.DecodedLen(len(work.PackageBaseDataBase64)))
	_, err = base64.StdEncoding.Decode(data, []byte(work.PackageBaseDataBase64))
	if err != nil {
		return err
	}

	if err := docker_client.CopyToContainer(ctx, containerId, "/home/ci/aur", bytes.NewReader(data), types.CopyToContainerOptions{}); err != nil {
		return err
	}

	// fix permissions
	_, exitCode, err = runAndWaitForExec(ctx, docker_client, containerId, types.ExecConfig{
		Cmd: []string{
			"chown", "-R", "ci:ci", "/home/ci",
		},
		AttachStdout: true,
	})
	if err != nil {
		return err
	}
	if exitCode > 0 {
		return err
	}

	return nil
}

func installDependencies(ctx context.Context, work *model.Work, containerId string) ([]byte, int, error) {
	log.Printf("[%s] Updating system and installing dependencies\n", work.PackageBase)

	cmd := []string{
		"pacman",
		"-Syu",
		"--noconfirm",
		"--noprogressbar",
		"--needed",
		"--", // We want to make sure no-one injects pacman parameters over the dependency list
	}
	cmd = append(cmd, work.Dependencies...)

	return runAndWaitForExec(ctx, docker_client, containerId, types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
	})
}

func downloadAndExtractPackage(ctx context.Context, work *model.Work, containerId string) ([]byte, int, error) {
	log.Printf("[%s] Downloading and extracting package sources\n", work.PackageBase)

	return runAndWaitForExec(ctx, docker_client, containerId, types.ExecConfig{
		Cmd: []string{
			"makepkg", "--nobuild",
		},
		User:         "ci",
		AttachStdout: true,
		WorkingDir:   "/home/ci/aur/" + work.PackageBase,
	})
}

func buildPackage(ctx context.Context, work *model.Work, containerId string) ([]byte, int, error) {
	log.Printf("[%s] Building package\n", work.PackageBase)

	return runAndWaitForExec(ctx, docker_client, containerId, types.ExecConfig{
		Cmd: []string{
			"makepkg", "--noextract",
		},
		User:         "ci",
		AttachStdout: true,
		WorkingDir:   "/home/ci/aur/" + work.PackageBase,
	})
}

func handleWork(waitgroup *sync.WaitGroup, work *model.Work) {
	defer waitgroup.Done()

	log.Printf("[%s] Handling work request\n", work.PackageBase)

	workResult := model.WorkResult{
		BuildId: work.BuildId,
		Status:  model.WORK_RESULT_STATUS_INTERNAL_ERROR,
	}
	defer sendWorkResult(&workResult, work.PackageBase)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	buildContainer, err := createContainer(ctx, work)
	if err != nil {
		return
	}

	defer docker_client.ContainerRemove(ctx, buildContainer.ID, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})

	log.Printf("[%s] Starting container ID %s\n", work.PackageBase, buildContainer.ID)

	if err := docker_client.ContainerStart(ctx, buildContainer.ID, types.ContainerStartOptions{}); err != nil {
		log.Printf("[%s] Failed to start container: %s\n", work.PackageBase, err)
		if err == context.DeadlineExceeded {
			workResult.Status = model.WORK_RESULT_STATUS_TIMEOUT
		} else {
			workResult.Status = model.WORK_RESULT_STATUS_INTERNAL_ERROR
		}
		return
	}

	err = prepareContainer(ctx, work, buildContainer.ID)
	if err != nil {
		log.Printf("[%s] Failed to prepare container: %s\n", work.PackageBase, err)
		if err == context.DeadlineExceeded {
			workResult.Status = model.WORK_RESULT_STATUS_TIMEOUT
		} else {
			workResult.Status = model.WORK_RESULT_STATUS_INTERNAL_ERROR
		}
		return
	}

	var pacmanLog []byte
	pacmanLog, workResult.PacmanExitCode, err = installDependencies(ctx, work, buildContainer.ID)

	if err != nil {
		log.Printf("[%s] Failed to install dependencies: %s\n", work.PackageBase, err)
		if err == context.DeadlineExceeded {
			workResult.Status = model.WORK_RESULT_STATUS_TIMEOUT
		} else {
			workResult.Status = model.WORK_RESULT_STATUS_INTERNAL_ERROR
		}
		return
	}
	workResult.PacmanLogBase64 = base64.StdEncoding.EncodeToString(pacmanLog)

	if workResult.PacmanExitCode > 0 {
		log.Printf("[%s] Pacman failed with exit code %d.\n", work.PackageBase, workResult.PacmanExitCode)
		workResult.Status = model.WORK_RESULT_STATUS_FAILED
		return
	}

	// TODO: Make filesystem diff before and after build

	var makepkgExtractLog []byte
	makepkgExtractLog, workResult.MakepkgExtractExitCode, err = downloadAndExtractPackage(ctx, work, buildContainer.ID)

	if err != nil {
		log.Printf("[%s] Failed to download and extract package: %s\n", work.PackageBase, err)
		if err == context.DeadlineExceeded {
			workResult.Status = model.WORK_RESULT_STATUS_TIMEOUT
		} else {
			workResult.Status = model.WORK_RESULT_STATUS_INTERNAL_ERROR
		}
		return
	}
	workResult.MakepkgExtractLogBase64 = base64.StdEncoding.EncodeToString(makepkgExtractLog)

	if workResult.MakepkgExtractExitCode > 0 {
		log.Printf("[%s] makepkg --nobuild failed with exit code %d.\n", work.PackageBase, workResult.MakepkgExtractExitCode)
		workResult.Status = model.WORK_RESULT_STATUS_FAILED
		return
	}

	var makepkgBuildLog []byte
	makepkgBuildLog, workResult.MakepkgBuildExitCode, err = buildPackage(ctx, work, buildContainer.ID)

	if err != nil {
		log.Printf("[%s] Failed to build package: %s\n", work.PackageBase, err)
		if err == context.DeadlineExceeded {
			workResult.Status = model.WORK_RESULT_STATUS_TIMEOUT
		} else {
			workResult.Status = model.WORK_RESULT_STATUS_INTERNAL_ERROR
		}
		return
	}
	workResult.MakepkgBuildLogBase64 = base64.StdEncoding.EncodeToString(makepkgBuildLog)

	if workResult.MakepkgBuildExitCode > 0 {
		log.Printf("[%s] makepkg --noextract failed with exit code %d.\n", work.PackageBase, workResult.MakepkgBuildExitCode)
		workResult.Status = model.WORK_RESULT_STATUS_FAILED
		return
	}

	workResult.Status = model.WORK_RESULT_STATUS_SUCCESS
}

func main() {
	workAmountEnvOrDefault, err := strconv.ParseUint(getEnv("WORK_AMOUNT", "1"), 10, 32)
	if err != nil {
		log.Fatal("Failed to parse $WORK_AMOUNT")
	}

	controller_uri = flag.String("controller", getEnv("CONTROLLER_URI", "http://127.0.0.1:8080"), "Controller URI")
	work_amount = flag.Int("work-amount", int(workAmountEnvOrDefault), "Amount of packages to build at once")
	flag.Parse()

	if len(*controller_uri) == 0 {
		log.Fatal("Missing controller URI")
	}

	initRootFSTARBuffer()
	initDockerClient()
	defer docker_client.Close()

	log.Printf("Sending initial heartbeat / registration to controller at %s", *controller_uri)
	err = sendHeartbeat()
	if err != nil {
		os.Exit(1)
	}

	c := cron.New()
	c.AddFunc("@every 1m", func() { _ = sendHeartbeat() })
	c.Start()

	log.Println("Registration successfull. Requesting work in a loop.")
	for {
		time.Sleep(time.Second)

		availableWorkload, err := requestWork()
		if err != nil {
			log.Println("Failed to request work: ", err)
			continue
		}

		if len(availableWorkload) == 0 {
			continue
		}

		pullArchLinuxImage()
		removeOldContainers()

		var waitgroup sync.WaitGroup
		waitgroup.Add(len(availableWorkload))

		for _, work := range availableWorkload {
			go handleWork(&waitgroup, &work)
		}

		waitgroup.Wait()
	}
}
