package main

import (
	"file-server/internal/api"
	"file-server/internal/config"
)

func main() {
	cfg := config.Load()
	api.StartServer(cfg)
}
