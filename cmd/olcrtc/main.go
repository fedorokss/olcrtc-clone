package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/app/session"
	configpkg "github.com/fedorokss/olcrtc-clone/internal/config"
	"github.com/fedorokss/olcrtc-clone/internal/logger"
	"github.com/fedorokss/olcrtc-clone/internal/names"
	"github.com/fedorokss/olcrtc-clone/internal/supervisor"
	protoLogger "github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

const modeGen = "gen"

var ErrConfigPathRequired = errors.New("usage: olcrtc <config.yaml>")

var ErrDataDirRequired = errors.New("data directory required (set 'data:' in YAML)")

var ErrProfilesUnsupportedForGen = errors.New("profiles are only supported for srv and cnc modes")

var runSession = session.Run

var runGen = execGen

type loadedConfig struct {
	scfg     session.Config
	profiles []supervisor.Profile
	failover failoverConfig
	dataDir  string
	debug    bool
}

type failoverConfig struct {
	retryDelay time.Duration
	maxCycles  int
}

func main() {
	if err := run(); err != nil {
		logger.Error(err)
		flushStderrFilter()
		os.Exit(1)
	}
}

func run() error {
	return runWithArgs(os.Args[1:])
}

func runWithArgs(args []string) error {
	logger.DisableNoisyPionLogs()
	installStderrFilter()
	session.RegisterDefaults()
	if len(args) != 1 || args[0] == "-h" || args[0] == "--help" || args[0] == "-help" {
		return ErrConfigPathRequired
	}
	cfg, err := loadConfig(args[0])
	if err != nil {
		return err
	}
	return runWithConfig(cfg)
}

func loadConfig(path string) (loadedConfig, error) {
	f, err := configpkg.Load(path)
	if err != nil {
		return loadedConfig{}, fmt.Errorf("load config: %w", err)
	}
	base := configpkg.Apply(session.Config{}, f)
	profiles := make([]supervisor.Profile, 0, len(f.Profiles))
	for i, profile := range f.Profiles {
		name := profile.Name
		if name == "" {
			name = fmt.Sprintf("profile-%d", i+1)
		}
		profiles = append(profiles, supervisor.Profile{
			Name:   name,
			Config: configpkg.ApplyProfile(base, profile),
		})
	}
	failover, err := parseFailoverConfig(f.Failover)
	if err != nil {
		return loadedConfig{}, err
	}
	return loadedConfig{
		scfg:     base,
		profiles: profiles,
		failover: failover,
		dataDir:  f.Data,
		debug:    f.Debug,
	}, nil
}

func parseFailoverConfig(f configpkg.Failover) (failoverConfig, error) {
	retryDelay := supervisor.DefaultRetryDelay
	if f.RetryDelay != "" {
		parsed, err := time.ParseDuration(f.RetryDelay)
		if err != nil {
			return failoverConfig{}, fmt.Errorf("parse failover.retry_delay: %w", err)
		}
		retryDelay = parsed
	}
	return failoverConfig{retryDelay: retryDelay, maxCycles: f.MaxCycles}, nil
}

func runWithConfig(cfg loadedConfig) error {
	configureLogging(cfg.debug)
	scfg, err := session.ApplyAuthDefaults(cfg.scfg)
	if err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	scfg = session.ApplyTransportDefaults(scfg)
	scfg = session.ApplyLivenessDefaults(scfg)
	if scfg.Mode == modeGen {
		if len(cfg.profiles) > 0 {
			return ErrProfilesUnsupportedForGen
		}
		return runGen(scfg)
	}
	if len(cfg.profiles) > 0 {
		profiles, err := prepareProfiles(cfg.profiles)
		if err != nil {
			return err
		}
		return runFailoverSessionMode(cfg.dataDir, profiles, cfg.failover)
	}
	return runSessionMode(cfg.dataDir, scfg)
}

func prepareProfiles(profiles []supervisor.Profile) ([]supervisor.Profile, error) {
	out := make([]supervisor.Profile, 0, len(profiles))
	for _, profile := range profiles {
		scfg, err := session.ApplyAuthDefaults(profile.Config)
		if err != nil {
			return nil, fmt.Errorf("validate profile %q: %w", profile.Name, err)
		}
		profile.Config = session.ApplyLivenessDefaults(session.ApplyTransportDefaults(scfg))
		out = append(out, profile)
	}
	return out, nil
}

func runSessionMode(dataDir string, scfg session.Config) error {
	if err := session.Validate(scfg); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	if err := prepareRuntimeData(dataDir); err != nil {
		return err
	}
	return runManaged(func(ctx context.Context) error {
		return runSession(ctx, scfg)
	})
}

func runFailoverSessionMode(dataDir string, profiles []supervisor.Profile, failover failoverConfig) error {
	for _, profile := range profiles {
		if err := session.Validate(profile.Config); err != nil {
			return fmt.Errorf("validate profile %q: %w", profile.Name, err)
		}
	}
	if err := prepareRuntimeData(dataDir); err != nil {
		return err
	}
	return runManaged(func(ctx context.Context) error {
		return supervisor.Run(ctx, supervisor.Config{
			Profiles:   profiles,
			RetryDelay: failover.retryDelay,
			MaxCycles:  failover.maxCycles,
			OnProfileStart: func(profile supervisor.Profile, cycle int) {
				logger.Infof("failover cycle=%d starting profile=%s carrier=%s transport=%s",
					cycle, profile.Name, profile.Config.Auth, profile.Config.Transport)
			},
			OnProfileEnd: func(profile supervisor.Profile, cycle int, err error) {
				if err != nil {
					logger.Warnf("failover cycle=%d profile=%s ended with error: %v", cycle, profile.Name, err)
					return
				}
				logger.Warnf("failover cycle=%d profile=%s ended", cycle, profile.Name)
			},
			OnStatus: logFailoverStatus,
		}, runSession)
	})
}

func logFailoverStatus(status supervisor.Status) {
	if !logger.IsVerbose() {
		return
	}
	active := status.ActiveProfile
	if active == "" {
		active = "none"
	}
	logger.Debugf("failover status cycle=%d active=%s last_error=%q profiles=%s history=%d",
		status.Cycle, active, status.LastError, formatProfileStatuses(status.Profiles), len(status.History))
}

func formatProfileStatuses(profiles []supervisor.ProfileStatus) string {
	if len(profiles) == 0 {
		return "[]"
	}
	var buf bytes.Buffer
	buf.Grow(len(profiles) * 48)
	buf.WriteByte('[')
	for i, profile := range profiles {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%s{starts=%d failures=%d clean=%d}",
			profile.Name, profile.Starts, profile.Failures, profile.CleanEnds)
	}
	buf.WriteByte(']')
	return buf.String()
}

func prepareRuntimeData(dataDir string) error {
	if dataDir == "" {
		return ErrDataDirRequired
	}
	resolvedDataDir, err := resolveDataDir(dataDir)
	if err != nil {
		return err
	}
	return loadNames(resolvedDataDir)
}

func runManaged(run func(context.Context) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx)
	}()
	select {
	case <-sigCh:
		logger.Info("Shutting down gracefully...")
		cancel()
		return waitForShutdown(errCh)
	case err := <-errCh:
		return err
	}
}

func execGen(scfg session.Config) error {
	if err := session.ValidateGen(scfg); err != nil {
		return fmt.Errorf("validate gen config: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Gen(ctx, scfg, func(id string) { _, _ = fmt.Fprintln(os.Stdout, id) })
	}()
	select {
	case <-sigCh:
		cancel()
		return waitForShutdown(errCh)
	case err := <-errCh:
		return err
	}
}

var noisyPrefixes = [][]byte{
	[]byte("turnc"), []byte("[turn]"), []byte("Fail to refresh permissions"),
}

type filteredWriter struct{ w io.Writer }

func (f filteredWriter) Write(p []byte) (int, error) {
	if isNoisyLogLine(p) {
		return len(p), nil
	}
	n, err := f.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("log write: %w", err)
	}
	return n, nil
}

func isNoisyLogLine(line []byte) bool {
	for _, prefix := range noisyPrefixes {
		if bytes.Contains(line, prefix) {
			return true
		}
	}
	return false
}

func configureLogging(debug bool) {
	installStderrFilter()
	log.SetOutput(filteredWriter{w: os.Stderr})
	logger.DisableNoisyPionLogs()
	if debug {
		logger.SetVerbose(true)
		return
	}
	_ = os.Setenv("PION_LOG_DISABLE", "all")
	lksdk.SetLogger(protoLogger.GetDiscardLogger())
}

func resolveDataDir(dataDir string) (string, error) {
	if filepath.IsAbs(dataDir) {
		return dataDir, nil
	}
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exePath), dataDir), nil
}

func loadNames(dataDir string) error {
	namesPath := filepath.Join(dataDir, "names")
	surnamesPath := filepath.Join(dataDir, "surnames")
	if err := names.LoadNameFiles(namesPath, surnamesPath); err != nil {
		return fmt.Errorf("load embedded names override: %w", err)
	}
	return nil
}

func waitForShutdown(errCh <-chan error) error {
	select {
	case err := <-errCh:
		if err == nil {
			logger.Info("Shutdown complete")
		}
		return err
	case <-time.After(5 * time.Second):
		logger.Warn("Shutdown timeout, forcing exit")
		return nil
	}
}
