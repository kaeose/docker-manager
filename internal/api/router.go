package api

import (
	"docker-manager/internal/web"
	"net/http"

	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(web.GetStaticFS())))

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/info", GetDockerInfo).Methods("GET")
	api.HandleFunc("/containers", GetContainers).Methods("GET")
	api.HandleFunc("/containers/{id}", GetContainerDetail).Methods("GET")
	api.HandleFunc("/containers/{id}/start", StartContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/stop", StopContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/restart", RestartContainer).Methods("POST")
	api.HandleFunc("/containers/{id}/logs", GetContainerLogs).Methods("GET")
	api.HandleFunc("/images", GetImages).Methods("GET")
	api.HandleFunc("/networks", GetNetworks).Methods("GET")
	api.HandleFunc("/volumes", GetVolumes).Methods("GET")
	api.HandleFunc("/system/stats", GetSystemStats).Methods("GET")
	api.HandleFunc("/system/events", GetSystemEvents).Methods("GET")
	api.HandleFunc("/system/host", GetHostSystemInfo).Methods("GET")

	// Systemd service management routes
	api.HandleFunc("/services", GetSystemdServices).Methods("GET")
	api.HandleFunc("/services/{name}", GetSystemdServiceDetail).Methods("GET")
	api.HandleFunc("/services/{name}/start", StartSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/stop", StopSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/restart", RestartSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/enable", EnableSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/disable", DisableSystemdService).Methods("POST")
	api.HandleFunc("/services/{name}/logs", GetSystemdServiceLogs).Methods("GET")

	// WebSocket for real-time updates
	r.HandleFunc("/ws", HandleWebSocket)

	// Serve index.html for root path
	r.HandleFunc("/", ServeIndex)

	return r
}
