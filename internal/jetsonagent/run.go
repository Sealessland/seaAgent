package jetsonagent

import (
	"errors"
	"fmt"
	"net/http"
)

func Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config failed: %w", err)
	}

	app, err := newApplication(cfg)
	if err != nil {
		return fmt.Errorf("build application failed: %w", err)
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

	return <-errCh
}
