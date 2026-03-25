package main

import "log"

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	server, err := newServer(cfg)
	if err != nil {
		log.Fatalf("build server failed: %v", err)
	}

	logStartup(cfg)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
