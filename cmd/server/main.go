package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"docker-manager/internal/api"
	"docker-manager/internal/service"
)

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

func main() {
	// Initialize Docker client
	service.InitDockerClient()

	port := getPort()
	r := api.NewRouter()

	fmt.Printf("Docker Manager starting on %s\n", port)
	log.Fatal(http.ListenAndServe(port, r))
}
