package service

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"docker-manager/internal/models"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
)

var DockerClient *client.Client
var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func InitDockerClient() {
	var err error
	DockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("Failed to create Docker client:", err)
	}
}

// Logic functions that use the docker client

func GetDockerInfo() (*models.DockerInfo, error) {
	ctx := context.Background()

	info, err := DockerClient.Info(ctx)
	if err != nil {
		return nil, err
	}

	version, err := DockerClient.ServerVersion(ctx)
	if err != nil {
		return nil, err
	}

	containers, err := DockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	images, err := DockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		return nil, err
	}

	networks, err := DockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	volumes, err := DockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}

	diskUsage, err := DockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return nil, err
	}

	return &models.DockerInfo{
		SystemInfo: &info,
		Version:    version,
		Containers: containers,
		Images:     images,
		Networks:   networks,
		Volumes:    volumes,
		DiskUsage:  diskUsage,
	}, nil
}

func GetSystemStats() (*models.SystemStats, error) {
	ctx := context.Background()

	containers, err := DockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	images, err := DockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		return nil, err
	}

	networks, err := DockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}

	volumes, err := DockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}

	stats := &models.SystemStats{}
	stats.Containers.Total = len(containers)
	for _, container := range containers {
		switch container.State {
		case "running":
			stats.Containers.Running++
		case "paused":
			stats.Containers.Paused++
		default:
			stats.Containers.Stopped++
		}
	}

	stats.Images.Total = int64(len(images))
	for _, img := range images {
		stats.Images.Size += img.Size
	}

	stats.Networks.Total = len(networks)
	stats.Volumes.Total = len(volumes.Volumes)

	return stats, nil
}

func GetContainerDetail(containerID string) (*models.ContainerDetail, error) {
	ctx := context.Background()
	containerJSON, err := DockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	detail := &models.ContainerDetail{
		Container: containerJSON,
	}

	// Get stats if container is running
	if containerJSON.State.Running {
		stats, err := DockerClient.ContainerStats(ctx, containerID, false)
		if err == nil {
			var statsJSON types.StatsJSON
			if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err == nil {
				detail.Stats = &statsJSON
			}
			stats.Body.Close()
		}
	}
	return detail, nil
}

func StartContainer(containerID string) error {
	ctx := context.Background()
	return DockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

func StopContainer(containerID string) error {
	ctx := context.Background()
	timeout := 10
	return DockerClient.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func RestartContainer(containerID string) error {
	ctx := context.Background()
	timeout := 10
	return DockerClient.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func StreamSystemEvents(ctx context.Context, since, until string, w http.ResponseWriter) error {
	options := types.EventsOptions{}
	if since != "" {
		if timestamp, err := strconv.ParseInt(since, 10, 64); err == nil {
			options.Since = strconv.FormatInt(timestamp, 10)
		}
	}
	if until != "" {
		if timestamp, err := strconv.ParseInt(until, 10, 64); err == nil {
			options.Until = strconv.FormatInt(timestamp, 10)
		}
	}

	events, errs := DockerClient.Events(ctx, options)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	encoder := json.NewEncoder(w)

	for {
		select {
		case event := <-events:
			encoder.Encode(event)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case err := <-errs:
			return err
		case <-ctx.Done():
			return nil
		}
	}
}
