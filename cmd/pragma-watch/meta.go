package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/artpar/pragma/internal/cli"
	pragmaconfig "github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/metaobserve"
	"github.com/artpar/pragma/internal/observe"
)

// metaCritic builds the model-backed critic: one small Complete call per
// sample through the same production provider construction pragma uses.
func metaCritic(cfg *config) (*metaobserve.ProviderCritic, error) {
	creds, err := pragmaconfig.LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	provName := cfg.criticProvider
	cred := creds.CredentialFor(provName)
	if cred.APIKey == "" && provName != "morphllm" {
		// morphllm.New falls back to MORPH_API_KEY itself; other providers
		// would fail later with a clearer provider-side error anyway.
		return nil, fmt.Errorf("no credential for critic provider %q", provName)
	}
	bus := observe.NewEventBus(0) // no logger: the critic must not create session logs
	prov, err := cli.CreateProvider(pragmaconfig.Config{Provider: provName, APIKey: cred.APIKey}, bus)
	if err != nil {
		return nil, fmt.Errorf("create critic provider: %w", err)
	}
	model := cfg.criticModel
	if model == "" {
		model = cli.DefaultModelFor(provName)
	}
	return &metaobserve.ProviderCritic{Prov: prov, Model: model, MaxTokens: 800}, nil
}

// metaObserverConfig assembles the observer config from CLI flags.
func metaObserverConfig(cfg *config, queuePath string) metaobserve.Config {
	interval := cfg.criticEvery
	if interval <= 0 {
		interval = 3 * time.Minute
	}
	active := cfg.activeWithin
	if active <= 0 {
		active = 15 * time.Minute
	}
	return metaobserve.Config{
		LogDir:          filepath.Join(cfg.home, "logs"),
		SessionsDir:     filepath.Join(cfg.home, "sessions"),
		QueuePath:       queuePath,
		FindingsPath:    filepath.Join(observationsDir(cfg), "meta-findings.jsonl"),
		Interval:        interval,
		FirstWindow:     10 * time.Minute,
		ActiveWithin:    active,
		MaxLogLines:     160,
		MaxSessionMsgs:  40,
		MaxDigestChars:  12000,
		MaxCallsPerHour: cfg.criticMaxCalls,
		MaxQueueItems:   200,
		CriticTimeout:   90 * time.Second,
		CriticProvider:  cfg.criticProvider,
		CriticModel:     cfg.criticModel,
	}
}

// runMeta runs the meta-observer loop in the foreground (Ctrl-C stops it).
func runMeta(cfg *config, queuePath string) error {
	critic, err := metaCritic(cfg)
	if err != nil {
		return err
	}
	o := metaobserve.NewObserver(metaObserverConfig(cfg, queuePath), critic, os.Stderr)
	fmt.Fprintf(os.Stderr, "pragma-watch meta-observe: sampling %s every %s (critic %s/%s), findings to %s",
		filepath.Join(cfg.home, "logs"), cfg.criticEvery, cfg.criticProvider, critic.Model,
		metaObserverConfig(cfg, queuePath).FindingsPath)
	if queuePath != "" {
		fmt.Fprintf(os.Stderr, " + queue %s", queuePath)
	}
	fmt.Fprintln(os.Stderr)
	o.Run(context.Background())
	return nil
}

// startMetaDaemon spawns the detached meta-observer daemon if none is
// running (pidfile meta.pid under the record dir). It mirrors the
// recording daemon: survives this terminal and every pragma session.
func startMetaDaemon(cfg *config, queuePath string) error {
	record := observationsDir(cfg)
	if err := os.MkdirAll(record, 0o755); err != nil {
		return fmt.Errorf("create record dir: %w", err)
	}
	pidfile := filepath.Join(record, "meta.pid")
	if pid, alive := daemonAlive(pidfile); alive {
		fmt.Printf("pragma-watch meta-observe daemon already running (pid %d)\n", pid)
		return nil
	}
	if _, err := metaCritic(cfg); err != nil {
		return err // fail fast: no credentials, no daemon
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	args := []string{
		"--meta-child",
		"--home", cfg.home,
		"--record", record,
		"--critic-provider", cfg.criticProvider,
		"--critic-every", cfg.criticEvery.String(),
		"--critic-max-calls", strconv.Itoa(cfg.criticMaxCalls),
		"--active", cfg.activeWithin.String(),
	}
	if cfg.criticModel != "" {
		args = append(args, "--critic-model", cfg.criticModel)
	}
	if queuePath != "" {
		args = append(args, "--queue", queuePath)
	}
	logf, err := os.OpenFile(filepath.Join(record, "meta.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open meta log: %w", err)
	}
	logf.Close() // detachSpawn reopens it for the child's stdio
	pid, err := detachSpawn(exe, args, filepath.Join(record, "meta.log"))
	if err != nil {
		return fmt.Errorf("spawn meta daemon: %w", err)
	}
	if err := os.WriteFile(pidfile, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("write meta pidfile: %w", err)
	}
	fmt.Printf("pragma-watch meta-observe daemon started (pid %d), findings to %s\n",
		pid, filepath.Join(record, "meta-findings.jsonl"))
	if queuePath != "" {
		fmt.Printf("findings also appended to queue: %s\n", queuePath)
	}
	fmt.Println("Stop it with: kill " + strconv.Itoa(pid))
	return nil
}
