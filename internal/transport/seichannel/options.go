package seichannel

import (
	"fmt"
	"time"

	"github.com/openlibrecommunity/olcrtc/internal/transport"
)

type Options struct {
	FPS          int
	BatchSize    int
	FragmentSize int
	AckTimeoutMS int
}

var defaultAckTimeoutMS = int(defaultAckTimeout / time.Millisecond)

func (Options) TransportOptions() {}

func (o Options) withDefaults() Options {
	if o.FPS <= 0 {
		o.FPS = defaultFPS
	}
	if o.BatchSize <= 0 {
		o.BatchSize = defaultBatchSize
	}
	if o.FragmentSize <= 0 {
		o.FragmentSize = defaultFragmentSize
	}
	if o.AckTimeoutMS <= 0 {
		o.AckTimeoutMS = defaultAckTimeoutMS
	}
	return o
}

func optionsFrom(cfg transport.Config) (Options, error) {
	if cfg.Options == nil {
		return Options{}, nil
	}
	if opts, ok := cfg.Options.(Options); ok {
		return opts, nil
	}
	return Options{}, fmt.Errorf("%w: seichannel: got %T", transport.ErrOptionsTypeMismatch, cfg.Options)
}
