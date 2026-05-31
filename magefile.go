//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

//nolint:gochecknoglobals
var Default = Help

const (
	buildDir = "build"
	ldflags  = "-s -w"
)

var (
	goexe  = mg.GoCmd()
	goos   = envOr("GOOS", runtime.GOOS)
	goarch = envOr("GOARCH", runtime.GOARCH)
)

func Help() error {
	return sh.RunV("mage", "-l")
}

func Check() {
	mg.SerialDeps(Build, Vet, Lint, TestFull)
}

func All() {
	mg.SerialDeps(Check, E2e)
}

func Nightly() {
	mg.SerialDeps(All, Stress)
}

func Everything() {
	mg.SerialDeps(Nightly, Soak, LocalSoak)
}

func Build() error {
	mg.Deps(Deps)
	return buildBinary("olcrtc", "./cmd/olcrtc", goos, goarch)
}

func Cross() error {
	mg.Deps(Deps)
	targets := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"windows", "amd64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"freebsd", "amd64"},
		{"freebsd", "arm64"},
		{"openbsd", "amd64"},
		{"openbsd", "arm64"},
	}
	if err := ensureBuildDir(); err != nil {
		return err
	}
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	wg.Add(len(targets))
	for i, t := range targets {
		go func(i int, os_, arch string) {
			defer wg.Done()
			errs[i] = buildBinary("olcrtc", "./cmd/olcrtc", os_, arch)
		}(i, t.os, t.arch)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	fmt.Printf("✅ built %d platform(s)\n", len(targets))
	return nil
}

func Mobile() error {
	if err := ensureTool("gomobile"); err != nil {
		return fmt.Errorf("gomobile not found: run 'go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init'")
	}
	if err := ensureBuildDir(); err != nil {
		return err
	}
	return sh.RunV("gomobile", "bind",
		"-target=android",
		"-androidapi", "21",
		"-ldflags", "-s -w -checklinkname=0",
		"-o", filepath.Join(buildDir, "olcrtc.aar"),
		"./mobile",
	)
}

func Docker() error {
	tag := envOr("DOCKER_TAG", "olcrtc:latest")
	return sh.RunV("docker", "build", "-t", tag, ".")
}

func Podman() error {
	tag := envOr("DOCKER_TAG", "olcrtc:latest")
	return sh.RunV("podman", "build", "-t", tag, ".")
}

func Vet() error {
	return sh.RunV(goexe, "vet", "./...")
}

func Lint() error {
	if err := ensureTool("golangci-lint"); err != nil {
		return fmt.Errorf("golangci-lint not found, install it:\n  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest")
	}
	return sh.RunV("golangci-lint", "run", "./...")
}

func Tidy() error {
	if err := sh.RunV(goexe, "mod", "tidy"); err != nil {
		return err
	}
	return sh.RunV(goexe, "mod", "verify")
}

func Test() error {
	return sh.RunV(goexe, "test", "-race", "-count=1", "-short", "./...")
}

func TestFull() error {
	return sh.RunV(goexe, "test", "-race", "-count=1", "-timeout", "10m", "./...")
}

func E2e() error {
	args := make([]string, 0, 16)
	args = append(args, "test", "-count=1", "-v", "-timeout", "30m",
		"./internal/e2e/...",
		"-olcrtc.real-e2e=true",
	)
	if carriers := os.Getenv("E2E_CARRIERS"); carriers != "" {
		args = append(args, "-olcrtc.real-carriers="+carriers)
	}
	if transports := os.Getenv("E2E_TRANSPORTS"); transports != "" {
		args = append(args, "-olcrtc.real-transports="+transports)
	}
	if timeout := os.Getenv("E2E_TIMEOUT"); timeout != "" {
		args = append(args, "-olcrtc.real-timeout="+timeout)
	}
	if os.Getenv("E2E_STRESS") != "" {
		args = append(args, "-olcrtc.stress=true")
		if d := os.Getenv("E2E_STRESS_DURATION"); d != "" {
			args = append(args, "-olcrtc.stress-duration="+d)
		}
	}
	return sh.RunV(goexe, args...)
}

func Stress() error {
	bulk := envOr("STRESS_BULK_DURATION", "15m")
	echo := envOr("STRESS_ECHO_DURATION", "15m")
	caseTO := envOr("STRESS_CASE_TIMEOUT", "35m")
	overall := envOr("STRESS_TIMEOUT", "6h")
	args := make([]string, 0, 16)
	args = append(args, "test", "-count=1", "-v",
		"-timeout", overall,
		"-run", "^TestRealProviderTransportStress$",
		"./internal/e2e/...",
		"-olcrtc.real-e2e=true",
		"-olcrtc.stress=true",
		"-olcrtc.stress-bulk-duration="+bulk,
		"-olcrtc.stress-duration="+echo,
		"-olcrtc.stress-case-timeout="+caseTO,
	)
	if carriers := os.Getenv("E2E_CARRIERS"); carriers != "" {
		args = append(args, "-olcrtc.real-carriers="+carriers)
	}
	if transports := os.Getenv("E2E_TRANSPORTS"); transports != "" {
		args = append(args, "-olcrtc.real-transports="+transports)
	}
	return sh.RunV(goexe, args...)
}

func Soak() error {
	carriers := envOr("SOAK_CARRIERS", "telemost,jitsi,wbstream")
	transports := envOr("SOAK_TRANSPORTS", "datachannel,vp8channel")
	duration := envOr("SOAK_DURATION", "10m")
	args := []string{"test", "-count=1", "-v",
		"-timeout", "12h",
		"-run", "^TestRealThroughputSoak$",
		"./internal/e2e/...",
		"-olcrtc.real-e2e=true",
		"-olcrtc.real-soak=true",
		"-olcrtc.real-soak-carrier=" + carriers,
		"-olcrtc.real-soak-transport=" + transports,
		"-olcrtc.real-soak-duration=" + duration,
	}
	return sh.RunV(goexe, args...)
}

func LocalSoak() error {
	transports := envOr("SOAK_TRANSPORTS", "all")
	duration := envOr("SOAK_DURATION", "6m")
	chaos := os.Getenv("SOAK_CHAOS")
	args := make([]string, 0, 10)
	args = append(args, "test", "-count=1", "-v",
		"-timeout", "12h",
		"-run", "^TestLocalThroughputSoak$",
		"./internal/e2e/...",
		"-olcrtc.local-soak=true",
		"-olcrtc.local-soak-transport="+transports,
		"-olcrtc.local-soak-duration="+duration,
	)
	if chaos != "" {
		args = append(args, "-olcrtc.local-soak-chaos="+chaos)
	}
	return sh.RunV(goexe, args...)
}

func Deps() error {
	return sh.RunV(goexe, "mod", "download")
}

func Clean() error {
	return os.RemoveAll(buildDir)
}

func buildBinary(name, pkg, os_, arch string) error {
	if err := ensureBuildDir(); err != nil {
		return err
	}
	ext := ""
	if os_ == "windows" {
		ext = ".exe"
	}
	out := filepath.Join(buildDir, fmt.Sprintf("%s-%s-%s%s", name, os_, arch, ext))
	fmt.Printf("building %s (%s/%s) -> %s\n", name, os_, arch, out)
	flags := ldflags
	if os_ == "android" {
		flags += " -checklinkname=0"
	}
	env := map[string]string{
		"GOOS":        os_,
		"GOARCH":      arch,
		"CGO_ENABLED": "0",
	}
	return sh.RunWithV(env, goexe, "build", "-trimpath", "-ldflags", flags, "-o", out, pkg)
}

func ensureBuildDir() error {
	return os.MkdirAll(buildDir, 0o755)
}

func ensureTool(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
