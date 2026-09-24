// Command server runs the TorrentTrackerBrowser backend: API + embedded UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/api"
	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
	"github.com/Encoded77/TorrentTrackerBrowser/server/ui"

	// Adapters register themselves with the core registry.
	_ "github.com/Encoded77/TorrentTrackerBrowser/server/engine/qbittorrent"
	_ "github.com/Encoded77/TorrentTrackerBrowser/server/engine/torbox"
	_ "github.com/Encoded77/TorrentTrackerBrowser/server/source/prowlarr"
	_ "github.com/Encoded77/TorrentTrackerBrowser/server/storage/path"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the YAML config file")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(*configPath); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := core.LoadConfig(configPath)
	if err != nil {
		return err
	}
	reg, err := core.BuildRegistry(cfg)
	if err != nil {
		srcs, engs, sts := core.RegisteredTypes()
		return fmt.Errorf("%w (known types: sources %v, engines %v, storages %v)", err, srcs, engs, sts)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("dataDir: %w", err)
	}
	store := core.NewJobStore(filepath.Join(cfg.DataDir, "jobs.json"), cfg.Limits.JobHistory)
	store.OnSaveError(func(err error) { slog.Error("jobs: save state", "err", err) })
	if err := store.Load(); err != nil {
		return fmt.Errorf("jobs.json: %w", err)
	}
	runner := core.NewRunner(reg, store, core.NewWebhook(cfg.Notify.Webhook), cfg.Limits)
	runner.Lang = cfg.Notify.Lang

	srv := &api.Server{
		Config:   cfg,
		Reg:      reg,
		Results:  core.NewResultCache(cfg.Limits.ResultTTL, cfg.Limits.ResultCap),
		Payloads: core.NewPayloadCache(cfg.Limits.ResultTTL, 1000),
		Searcher: &core.Searcher{Sources: reg.Sources, IndexerTimeout: cfg.Limits.IndexerTimeout, PerIndexer: cfg.Limits.ResultsPerIndexer},
		Runner:   runner,
	}
	srv.Searcher.Results = srv.Results

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner.Start(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(ui.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen, "sources", len(reg.Sources), "engines", len(reg.Engines), "storages", len(reg.Storages))
		errCh <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	runner.Stop()
	if err := store.Flush(); err != nil {
		slog.Error("jobs: flush state", "err", err)
	}
	return nil
}
