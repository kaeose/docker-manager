package models

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
)

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
