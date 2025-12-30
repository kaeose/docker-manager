package service

import (
	"bufio"
	"docker-manager/internal/models"
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

func GetHostSystemInfo() (*models.HostSystemInfo, error) {
	hostInfo := &models.HostSystemInfo{}

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

	return hostInfo, nil
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

func GetSystemdServices() ([]models.SystemdService, error) {
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var services []models.SystemdService
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse systemctl output
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			service := models.SystemdService{
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

	return services, nil
}

func GetSystemdServiceDetail(serviceName string) (*models.SystemdServiceDetail, error) {
	// Get service status
	statusCmd := exec.Command("systemctl", "status", serviceName, "--no-pager", "--lines=0")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse status output
	service := models.SystemdService{Name: serviceName}
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

	detail := &models.SystemdServiceDetail{
		Service: service,
		Status:  statusStr,
		Logs:    logs,
		Props:   properties,
	}

	return detail, nil
}
