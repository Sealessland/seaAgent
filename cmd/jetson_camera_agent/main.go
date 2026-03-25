package main

import (
	"errors"
	"log"
	"net/http"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	app, err := newApplication(cfg)
	if err != nil {
		log.Fatalf("build application failed: %v", err)
	}

	logStartup(cfg)
	apiServer := app.newAPIServer()
	debugServer := app.newDebugServer()

	errCh := make(chan error, 2)
	go func() {
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		if err := debugServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	if err := <-errCh; err != nil {
		log.Fatal(err)
	}
}
