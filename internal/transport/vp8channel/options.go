package vp8channel

import (
	"fmt"

	"github.com/openlibrecommunity/olcrtc/internal/transport"
)

const (
	defaultFPS            = 60
	defaultBatchSize      = 64
	defaultMaxBytesPerSec = 1_200_000
)

type Options struct {
	FPS            int
	BatchSize      int
	MaxBytesPerSec int
}

func (Options) TransportOptions() {}

func optionsFrom(cfg transport.Config) (Options, error) {
	if cfg.Options == nil {
		return Options{}, nil
	}
	opts, ok := cfg.Options.(Options)
	if !ok {
		return Options{}, fmt.Errorf("%w: vp8channel: got %T", transport.ErrOptionsTypeMismatch, cfg.Options)
	}
	return opts, nil
}
