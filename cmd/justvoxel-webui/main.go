package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/home-server-project/justvoxel-webui/internal/api"
	"github.com/home-server-project/justvoxel-webui/internal/server"
	"github.com/home-server-project/justvoxel-webui/internal/version"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:8443", "HTTPS listen address")
	socket := flag.String("agent-socket", "/run/justvoxel/management.sock", "JustVoxel management Unix socket")
	cert := flag.String("tls-cert", "/run/credentials/justvoxel-webui.service/tls.crt", "TLS certificate path")
	key := flag.String("tls-key", "/run/credentials/justvoxel-webui.service/tls.key", "TLS private key path")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("JustVoxel WebUI %s\nsource=%s\nmanagement-api=%s\n", version.Version, version.Commit, version.ManagementAPI)
		return
	}

	client := api.NewClient(*socket)
	app, err := server.New(client, server.Config{
		Version:       version.Version,
		Commit:        version.Commit,
		ManagementAPI: version.ManagementAPI,
	})
	if err != nil {
		log.Fatalf("initialize web server: %v", err)
	}

	log.Printf("JustVoxel WebUI %s listening on %s", version.Version, *listen)
	if err := app.ListenAndServeTLS(*listen, *cert, *key); err != nil {
		log.Printf("web server stopped: %v", err)
		os.Exit(1)
	}
}
