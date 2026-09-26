// exe-planet builds Planet-compatible static sites from Markdown in the
// exe Workspace. This is the daemon; the desk reaches it through exe's
// service relay on the loopback port below.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/livid/exe-planet/internal/api"
	"github.com/livid/exe-planet/internal/service"
	"github.com/livid/exe-planet/internal/site"
	"github.com/livid/exe-planet/internal/watch"
)

const version = "0.4.0"

func main() {
	home, _ := os.UserHomeDir()
	listen := flag.String("listen", "127.0.0.1:7799", "address to serve the API on")
	workspace := flag.String("workspace", filepath.Join(home, ".exe", "workspace", "Planet"), "the Planet folder in the Workspace: sites, templates, archived sites")
	state := flag.String("state", filepath.Join(home, ".exe", "planet"), "per-node state: build output, node settings, keys")
	templates := flag.String("templates", "templates", "the shipped templates, installed into the Workspace's Planet/Templates")
	flag.Parse()

	if err := run(*listen, *workspace, *state, *templates); err != nil {
		log.Fatal(err)
	}
}

func run(listen, workspace, state, templates string) error {
	if err := os.MkdirAll(state, 0o700); err != nil {
		return err
	}
	store := site.New(workspace)
	if err := store.Init(); err != nil {
		return err
	}
	if err := service.InstallTemplates(templates, filepath.Join(workspace, site.TemplatesDir)); err != nil {
		log.Printf("templates: %v", err)
	}
	svc := service.New(store, state, time.Local)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	poller := &watch.Poller{Root: workspace, OnChange: func(paths []string) {
		shown := paths
		if len(shown) > 6 {
			shown = append(shown[:6:6], fmt.Sprintf("… %d more", len(paths)-6))
		}
		log.Printf("workspace changed: %s", strings.Join(shown, ", "))
		svc.OnChange(paths)
	}}
	go poller.Run(ctx)
	go svc.BuildAll()

	srv := &http.Server{
		Addr:              listen,
		Handler:           (&api.Server{Store: store, Builder: svc, Templates: svc.Templates, Version: version, Started: time.Now(), Generation: poller.Generation}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("exe-planet %s serving %s from %s", version, listen, workspace)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}
