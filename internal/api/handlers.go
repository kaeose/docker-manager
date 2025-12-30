package api

import (
	"context"
	"docker-manager/internal/service"
	"docker-manager/internal/web"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
	"github.com/gorilla/mux"
)

func ServeIndex(w http.ResponseWriter, r *http.Request) {
	data, err := web.ReadIndex()
	if err != nil {
		http.Error(w, "Could not read index.html", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func GetDockerInfo(w http.ResponseWriter, r *http.Request) {
	info, err := service.GetDockerInfo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func GetContainers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	containers, err := service.DockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

func GetContainerDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	detail, err := service.GetContainerDetail(containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func StartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	err := service.StartContainer(containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func StopContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	err := service.StopContainer(containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func RestartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	err := service.RestartContainer(containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "restarted"})
}

func GetContainerLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	ctx := context.Background()
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Timestamps: true,
	}

	logs, err := service.DockerClient.ContainerLogs(ctx, containerID, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer logs.Close()

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Transfer-Encoding", "chunked")

	buffer := make([]byte, 4096)
	for {
		n, err := logs.Read(buffer)
		if err != nil {
			break
		}
		w.Write(buffer[:n])
	}
}

func GetImages(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	images, err := service.DockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

func GetNetworks(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	networks, err := service.DockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networks)
}

func GetVolumes(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	volumes, err := service.DockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(volumes)
}

func GetSystemStats(w http.ResponseWriter, r *http.Request) {
	stats, err := service.GetSystemStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func GetSystemEvents(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	since := r.URL.Query().Get("since")
	until := r.URL.Query().Get("until")

	err := service.StreamSystemEvents(ctx, since, until, w)
	if err != nil {
		return
	}
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := service.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	ctx := context.Background()
	events, errs := service.DockerClient.Events(ctx, types.EventsOptions{})

	for {
		select {
		case event := <-events:
			if err := conn.WriteJSON(event); err != nil {
				log.Println("WebSocket write error:", err)
				return
			}
		case err := <-errs:
			if err != nil {
				log.Println("Docker events error:", err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func GetHostSystemInfo(w http.ResponseWriter, r *http.Request) {
	hostInfo, err := service.GetHostSystemInfo()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get host info: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hostInfo)
}

func GetSystemdServices(w http.ResponseWriter, r *http.Request) {
	services, err := service.GetSystemdServices()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get services: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func GetSystemdServiceDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	detail, err := service.GetSystemdServiceDetail(serviceName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get service detail: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func StartSystemdService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	cmd := exec.Command("systemctl", "start", serviceName)
	err := cmd.Run()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Service started"})
}

func StopSystemdService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	cmd := exec.Command("systemctl", "stop", serviceName)
	err := cmd.Run()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Service stopped"})
}

func RestartSystemdService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	cmd := exec.Command("systemctl", "restart", serviceName)
	err := cmd.Run()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to restart service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Service restarted"})
}

func EnableSystemdService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	cmd := exec.Command("systemctl", "enable", serviceName)
	err := cmd.Run()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to enable service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Service enabled"})
}

func DisableSystemdService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	cmd := exec.Command("systemctl", "disable", serviceName)
	err := cmd.Run()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to disable service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Service disabled"})
}

func GetSystemdServiceLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	// Get query parameters
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "100"
	}

	follow := r.URL.Query().Get("follow") == "true"

	var cmd *exec.Cmd
	if follow {
		cmd = exec.Command("journalctl", "-u", serviceName, "--no-pager", "-n", lines, "-f", "--output=short")
	} else {
		cmd = exec.Command("journalctl", "-u", serviceName, "--no-pager", "-n", lines, "--output=short")
	}

	output, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get service logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(output)
}
