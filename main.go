package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

var dockerClient *client.Client
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// getPort returns the port to listen on
func getPort() string {
	// Priority: 1. Command line flag, 2. Environment variable, 3. Default
	var port = flag.String("port", "", "Port to listen on (default: 8080)")
	flag.Parse()

	if *port != "" {
		return ":" + *port
	}

	if envPort := os.Getenv("DOCKER_MANAGER_PORT"); envPort != "" {
		return ":" + envPort
	}

	return ":8080"
}

type DockerInfo struct {
	SystemInfo *types.Info             `json:"system_info"`
	Version    types.Version           `json:"version"`
	Containers []types.Container       `json:"containers"`
	Images     []types.ImageSummary    `json:"images"`
	Networks   []types.NetworkResource `json:"networks"`
	Volumes    volume.ListResponse     `json:"volumes"`
	DiskUsage  types.DiskUsage         `json:"disk_usage"`
}

type ContainerDetail struct {
	Container types.ContainerJSON `json:"container"`
	Stats     *types.StatsJSON    `json:"stats,omitempty"`
}

type SystemStats struct {
	Containers struct {
		Running int `json:"running"`
		Paused  int `json:"paused"`
		Stopped int `json:"stopped"`
		Total   int `json:"total"`
	} `json:"containers"`
	Images struct {
		Total int64 `json:"total"`
		Size  int64 `json:"size"`
	} `json:"images"`
	Networks struct {
		Total int `json:"total"`
	} `json:"networks"`
	Volumes struct {
		Total int `json:"total"`
	} `json:"volumes"`
}

type HostSystemInfo struct {
	Uptime             string  `json:"uptime"`
	UptimeSeconds      int64   `json:"uptime_seconds"`
	LoadAverage1       float64 `json:"load_avg_1"`
	LoadAverage5       float64 `json:"load_avg_5"`
	LoadAverage15      float64 `json:"load_avg_15"`
	MemoryTotal        int64   `json:"memory_total"`
	MemoryUsed         int64   `json:"memory_used"`
	MemoryAvailable    int64   `json:"memory_available"`
	MemoryUsedPct      float64 `json:"memory_used_percent"`
	NetworkConnections int     `json:"network_connections"`
	CPUCores           int     `json:"cpu_cores"`
}

// SystemdService represents a systemd service
type SystemdService struct {
	Name        string `json:"name"`
	LoadState   string `json:"load_state"`
	ActiveState string `json:"active_state"`
	SubState    string `json:"sub_state"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
	Type        string `json:"type"`
	MainPID     string `json:"main_pid"`
	Memory      string `json:"memory"`
	Tasks       string `json:"tasks"`
}

// SystemdServiceDetail represents detailed information about a systemd service
type SystemdServiceDetail struct {
	Service SystemdService    `json:"service"`
	Status  string            `json:"status"`
	Logs    []string          `json:"logs"`
	Props   map[string]string `json:"properties"`
}

func init() {
	var err error
	dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("Failed to create Docker client:", err)
	}
}

func main() {
	port := getPort()

	r := mux.NewRouter()

	// Static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/info", getDockerInfo).Methods("GET")
	api.HandleFunc("/containers", getContainers).Methods("GET")
	api.HandleFunc("/containers/{id}", getContainerDetail).Methods("GET")
	api.HandleFunc("/containers/{id}/start", startContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/stop", stopContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/restart", restartContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/logs", getContainerLogs).Methods("GET")
	api.HandleFunc("/images", getImages).Methods("GET")
	api.HandleFunc("/networks", getNetworks).Methods("GET")
	api.HandleFunc("/volumes", getVolumes).Methods("GET")
	api.HandleFunc("/system/stats", getSystemStats).Methods("GET")
	api.HandleFunc("/system/events", getSystemEvents).Methods("GET")
	api.HandleFunc("/system/host", getHostSystemInfo).Methods("GET")

	// Systemd service management routes
	api.HandleFunc("/services", getSystemdServices).Methods("GET")
	api.HandleFunc("/services/{name}", getSystemdServiceDetail).Methods("GET")
	api.HandleFunc("/services/{name}/start", startSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/stop", stopSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/restart", restartSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/enable", enableSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/disable", disableSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/logs", getSystemdServiceLogs).Methods("GET")

	// WebSocket for real-time updates
	r.HandleFunc("/ws", handleWebSocket)

	// Serve index.html for root path
	r.HandleFunc("/", serveIndex)

	fmt.Printf("Docker Manager starting on %s\n", port)
	log.Fatal(http.ListenAndServe(port, r))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Could not read index.html", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func getDockerInfo(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	info, err := dockerClient.Info(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	version, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	images, err := dockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	networks, err := dockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	diskUsage, err := dockerClient.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dockerInfo := DockerInfo{
		SystemInfo: &info,
		Version:    version,
		Containers: containers,
		Images:     images,
		Networks:   networks,
		Volumes:    volumes,
		DiskUsage:  diskUsage,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dockerInfo)
}

func getContainers(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(containers)
}

func getContainerDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	ctx := context.Background()
	containerJSON, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	detail := ContainerDetail{
		Container: containerJSON,
	}

	// Get stats if container is running
	if containerJSON.State.Running {
		stats, err := dockerClient.ContainerStats(ctx, containerID, false)
		if err == nil {
			var statsJSON types.StatsJSON
			if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err == nil {
				detail.Stats = &statsJSON
			}
			stats.Body.Close()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func startContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	ctx := context.Background()
	err := dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func stopContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	ctx := context.Background()
	timeout := 10
	err := dockerClient.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func restartContainer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	ctx := context.Background()
	timeout := 10
	err := dockerClient.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "restarted"})
}

func getContainerLogs(w http.ResponseWriter, r *http.Request) {
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

	logs, err := dockerClient.ContainerLogs(ctx, containerID, options)
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

func getImages(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	images, err := dockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(images)
}

func getNetworks(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	networks, err := dockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networks)
}

func getVolumes(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(volumes)
}

func getSystemStats(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	images, err := dockerClient.ImageList(ctx, types.ImageListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	networks, err := dockerClient.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stats := SystemStats{}
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func getSystemEvents(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	since := r.URL.Query().Get("since")
	until := r.URL.Query().Get("until")

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

	events, errs := dockerClient.Events(ctx, options)

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
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	ctx := context.Background()
	events, errs := dockerClient.Events(ctx, types.EventsOptions{})

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

func getHostSystemInfo(w http.ResponseWriter, r *http.Request) {
	hostInfo := HostSystemInfo{}

	// Get uptime
	if uptimeData, err := ioutil.ReadFile("/proc/uptime"); err == nil {
		uptimeStr := strings.TrimSpace(string(uptimeData))
		if uptimeParts := strings.Split(uptimeStr, " "); len(uptimeParts) > 0 {
			if uptimeSeconds, err := strconv.ParseFloat(uptimeParts[0], 64); err == nil {
				hostInfo.UptimeSeconds = int64(uptimeSeconds)
				hostInfo.Uptime = formatUptime(int64(uptimeSeconds))
			}
		}
	}

	// Get load average
	if loadData, err := ioutil.ReadFile("/proc/loadavg"); err == nil {
		loadStr := strings.TrimSpace(string(loadData))
		loadParts := strings.Split(loadStr, " ")
		if len(loadParts) >= 3 {
			if load1, err := strconv.ParseFloat(loadParts[0], 64); err == nil {
				hostInfo.LoadAverage1 = load1
			}
			if load5, err := strconv.ParseFloat(loadParts[1], 64); err == nil {
				hostInfo.LoadAverage5 = load5
			}
			if load15, err := strconv.ParseFloat(loadParts[2], 64); err == nil {
				hostInfo.LoadAverage15 = load15
			}
		}
	}

	// Get memory info
	if memData, err := ioutil.ReadFile("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(memData)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				if parts := strings.Fields(line); len(parts) >= 2 {
					if total, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						hostInfo.MemoryTotal = total * 1024 // Convert from KB to bytes
					}
				}
			} else if strings.HasPrefix(line, "MemAvailable:") {
				if parts := strings.Fields(line); len(parts) >= 2 {
					if available, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						hostInfo.MemoryAvailable = available * 1024 // Convert from KB to bytes
					}
				}
			}
		}
		hostInfo.MemoryUsed = hostInfo.MemoryTotal - hostInfo.MemoryAvailable
		if hostInfo.MemoryTotal > 0 {
			hostInfo.MemoryUsedPct = float64(hostInfo.MemoryUsed) / float64(hostInfo.MemoryTotal) * 100
		}
	}

	// Get CPU cores
	hostInfo.CPUCores = runtime.NumCPU()

	// Get network connections (simplified)
	if netData, err := ioutil.ReadFile("/proc/net/tcp"); err == nil {
		lines := strings.Split(string(netData), "\n")
		hostInfo.NetworkConnections = len(lines) - 2 // Subtract header and last empty line
		if hostInfo.NetworkConnections < 0 {
			hostInfo.NetworkConnections = 0
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hostInfo)
}

func formatUptime(seconds int64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

// getSystemdServices returns a list of systemd services
func getSystemdServices(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get services: %v", err), http.StatusInternalServerError)
		return
	}

	var services []SystemdService
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse systemctl output
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			service := SystemdService{
				Unit:        fields[0],
				Name:        strings.TrimSuffix(fields[0], ".service"),
				LoadState:   fields[1],
				ActiveState: fields[2],
				SubState:    fields[3],
			}

			// Get description if available
			if len(fields) > 4 {
				service.Description = strings.Join(fields[4:], " ")
			}

			services = append(services, service)
		}
	}

	// Sort services: running first, then by sub_state alphabetically
	sort.Slice(services, func(i, j int) bool {
		// Priority order: running, then other states alphabetically
		if services[i].SubState == "running" && services[j].SubState != "running" {
			return true
		}
		if services[i].SubState != "running" && services[j].SubState == "running" {
			return false
		}
		// If both are running or both are not running, sort by sub_state
		if services[i].SubState == services[j].SubState {
			// If sub_state is same, sort by name
			return services[i].Name < services[j].Name
		}
		return services[i].SubState < services[j].SubState
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// getSystemdServiceDetail returns detailed information about a specific service
func getSystemdServiceDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]

	// Get service status
	statusCmd := exec.Command("systemctl", "status", serviceName, "--no-pager", "--lines=0")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get service status: %v", err), http.StatusInternalServerError)
		return
	}

	// Parse status output
	service := SystemdService{Name: serviceName}
	statusStr := string(statusOutput)

	// Extract basic info from status output
	lines := strings.Split(statusStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Loaded:") {
			parts := strings.Split(line, ";")
			if len(parts) > 0 {
				loadPart := strings.TrimSpace(strings.TrimPrefix(parts[0], "Loaded:"))
				service.LoadState = strings.Fields(loadPart)[0]
			}
		} else if strings.Contains(line, "Active:") {
			activeLine := strings.TrimPrefix(line, "Active:")
			activeFields := strings.Fields(strings.TrimSpace(activeLine))
			if len(activeFields) >= 2 {
				service.ActiveState = activeFields[0]
				service.SubState = strings.Trim(activeFields[1], "()")
			}
		} else if strings.Contains(line, "Main PID:") {
			parts := strings.Fields(line)
			for i, part := range parts {
				if part == "PID:" && i+1 < len(parts) {
					service.MainPID = parts[i+1]
					break
				}
			}
		}
	}

	// Get service properties
	showCmd := exec.Command("systemctl", "show", serviceName, "--no-pager")
	showOutput, err := showCmd.Output()
	properties := make(map[string]string)

	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(showOutput)))
		for scanner.Scan() {
			line := scanner.Text()
			if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
				properties[parts[0]] = parts[1]
			}
		}

		// Extract useful properties
		if desc, ok := properties["Description"]; ok {
			service.Description = desc
		}
		if serviceType, ok := properties["Type"]; ok {
			service.Type = serviceType
		}
		if memory, ok := properties["MemoryCurrent"]; ok {
			service.Memory = memory
		}
		if tasks, ok := properties["TasksCurrent"]; ok {
			service.Tasks = tasks
		}
	}

	// Get recent logs
	logsCmd := exec.Command("journalctl", "-u", serviceName, "--no-pager", "-n", "50", "--output=short")
	logsOutput, _ := logsCmd.Output()

	var logs []string
	if logsOutput != nil {
		logLines := strings.Split(string(logsOutput), "\n")
		for _, line := range logLines {
			if strings.TrimSpace(line) != "" {
				logs = append(logs, line)
			}
		}
	}

	detail := SystemdServiceDetail{
		Service: service,
		Status:  statusStr,
		Logs:    logs,
		Props:   properties,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

// startSystemdService starts a systemd service
func startSystemdService(w http.ResponseWriter, r *http.Request) {
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

// stopSystemdService stops a systemd service
func stopSystemdService(w http.ResponseWriter, r *http.Request) {
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

// restartSystemdService restarts a systemd service
func restartSystemdService(w http.ResponseWriter, r *http.Request) {
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

// enableSystemdService enables a systemd service
func enableSystemdService(w http.ResponseWriter, r *http.Request) {
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

// disableSystemdService disables a systemd service
func disableSystemdService(w http.ResponseWriter, r *http.Request) {
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

// getSystemdServiceLogs returns logs for a specific service
func getSystemdServiceLogs(w http.ResponseWriter, r *http.Request) {
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
