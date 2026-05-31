package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/auth"
	"github.com/fedorokss/olcrtc-clone/internal/client"
	"github.com/fedorokss/olcrtc-clone/internal/control"
	enginebuiltin "github.com/fedorokss/olcrtc-clone/internal/engine/builtin"
	"github.com/fedorokss/olcrtc-clone/internal/logger"
	"github.com/fedorokss/olcrtc-clone/internal/names"
	"github.com/fedorokss/olcrtc-clone/internal/runtime"
	"github.com/fedorokss/olcrtc-clone/internal/server"
	"github.com/fedorokss/olcrtc-clone/internal/transport"
	"github.com/fedorokss/olcrtc-clone/internal/transport/datachannel"
	"github.com/fedorokss/olcrtc-clone/internal/transport/seichannel"
	"github.com/fedorokss/olcrtc-clone/internal/transport/videochannel"
	"github.com/fedorokss/olcrtc-clone/internal/transport/vp8channel"
)

const (
	modeSRV          = "srv"
	modeCNC          = "cnc"
	modeGen          = "gen"
	authNone         = "none"
	transportVideo   = "videochannel"
	transportVP8     = "vp8channel"
	transportSEI     = "seichannel"
	videoCodecQRCode = "qrcode"
	videoCodecTile   = "tile"
)

const (
	defaultVideoWidth      = 1920
	defaultVideoHeight     = 1080
	defaultVideoFPS        = 30
	defaultVideoBitrate    = "2M"
	defaultVideoHW         = "none"
	defaultVideoQRRecovery = "low"
	defaultVP8FPS          = 60
	defaultVP8BatchSize    = 64
	defaultSEIFPS          = 60
	defaultSEIBatchSize    = 64
	defaultSEIFragmentSize = 900
	defaultSEIAckTimeoutMS = 2000
)

var sessionRestartDelay = 2 * time.Second

var (
	ErrRoomIDRequired = errors.New("room ID required (set room.id)")
	ErrModeRequired   = errors.New("mode required (set mode to srv, cnc or gen)")
	ErrAmountRequired = errors.New("amount required for gen mode (set gen.amount)")
	ErrAuthRequired   = errors.New(
		"auth provider required (set auth.provider to jitsi, telemost, wbstream or none)")
	ErrURLRequired          = errors.New("SFU URL required (set auth.url)")
	ErrUnsupportedCarrier   = errors.New("unsupported carrier")
	ErrUnsupportedTransport = errors.New("unsupported transport")
	ErrTransportRequired    = errors.New(
		"transport required (set transport to datachannel, videochannel, seichannel or vp8channel)")
	ErrKeyRequired          = errors.New("key required (set crypto.key)")
	ErrDNSServerRequired    = errors.New("dns server required (set net.dns)")
	ErrVideoWidthRequired   = errors.New("video width required for videochannel (set video.width)")
	ErrVideoHeightRequired  = errors.New("video height required for videochannel (set video.height)")
	ErrVideoFPSRequired     = errors.New("video fps required for videochannel (set video.fps)")
	ErrVideoBitrateRequired = errors.New(
		"video bitrate required for videochannel (set video.bitrate)")
	ErrVideoHWRequired = errors.New(
		"video hardware acceleration required for videochannel (set video.hw to none or nvenc)")
	ErrVideoCodecInvalid = errors.New(
		"invalid video codec for videochannel (set video.codec to qrcode or tile)")
	ErrTileCodecDimensions     = errors.New("tile codec requires video.width: 1080 and video.height: 1080")
	ErrVP8FPSRequired          = errors.New("vp8 fps required for vp8channel (set vp8.fps)")
	ErrVP8BatchSizeRequired    = errors.New("vp8 batch size required for vp8channel (set vp8.batch_size)")
	ErrSEIFPSRequired          = errors.New("fps required for seichannel (set sei.fps)")
	ErrSEIBatchSizeRequired    = errors.New("batch size required for seichannel (set sei.batch_size)")
	ErrSEIFragmentSizeRequired = errors.New("fragment size required for seichannel (set sei.fragment_size)")
	ErrSEIAckTimeoutRequired   = errors.New("ack timeout required for seichannel (set sei.ack_timeout_ms)")
	ErrSOCKSHostRequired       = errors.New("socks host required for cnc mode (set socks.host)")
	ErrSOCKSPortRequired       = errors.New("socks port required for cnc mode (set socks.port)")
	ErrSOCKSAuthRequired       = errors.New(
		"socks auth required when binding outside loopback (set socks.user and socks.pass)")
	ErrLivenessIntervalInvalid = errors.New(
		"invalid liveness interval (set liveness.interval to a duration > 0)")
	ErrLivenessTimeoutInvalid = errors.New(
		"invalid liveness timeout (set liveness.timeout to a duration > 0)")
	ErrLivenessFailuresInvalid = errors.New(
		"invalid liveness failures (set liveness.failures to a value > 0)")
	ErrLifecycleMaxSessionDurationInvalid = errors.New(
		"invalid max session duration (set lifecycle.max_session_duration to a duration > 0)")
	ErrTrafficMaxPayloadSizeInvalid = errors.New(
		"invalid traffic max payload size (set traffic.max_payload_size to 0 or a value above crypto overhead)")
	ErrTrafficMinDelayInvalid = errors.New(
		"invalid traffic min delay (set traffic.min_delay to a duration >= 0)")
	ErrTrafficMaxDelayInvalid = errors.New(
		"invalid traffic max delay (set traffic.max_delay to a duration >= 0 and >= traffic.min_delay)")
	errPositiveDuration    = errors.New("duration must be > 0")
	errNonNegativeDuration = errors.New("duration must be >= 0")
)

type VideoConfig struct {
	Width      int
	Height     int
	FPS        int
	Bitrate    string
	HW         string
	QRSize     int
	QRRecovery string
	Codec      string
	TileModule int
	TileRS     int
}

type VP8Config struct {
	FPS       int
	BatchSize int
}

type SEIConfig struct {
	FPS          int
	BatchSize    int
	FragmentSize int
	AckTimeoutMS int
}

type Config struct {
	Mode                  string
	Transport             string
	Auth                  string
	WBToken               string
	WBCookie              string
	Engine                string
	URL                   string
	Token                 string
	RoomID                string
	ChannelID             string
	KeyHex                string
	SOCKSHost             string
	SOCKSPort             int
	SOCKSUser             string
	SOCKSPass             string
	DNSServer             string
	SOCKSProxyAddr        string
	SOCKSProxyPort        int
	SOCKSProxyUser        string
	SOCKSProxyPass        string
	Video                 VideoConfig
	VP8                   VP8Config
	SEI                   SEIConfig
	LivenessInterval      string
	LivenessTimeout       string
	LivenessFailures      int
	MaxSessionDuration    string
	TrafficMaxPayloadSize int
	TrafficMinDelay       string
	TrafficMaxDelay       string
	Amount                int
}

func RegisterDefaults() {
	enginebuiltin.RegisterDefaults()
	transport.Register("datachannel", datachannel.New)
	transport.Register("videochannel", videochannel.New)
	transport.Register("seichannel", seichannel.New)
	transport.Register("vp8channel", vp8channel.New)
}

func ApplyAuthDefaults(cfg Config) (Config, error) {
	if cfg.Auth == authNone || cfg.Auth == "" {
		return cfg, nil
	}
	p, _ := auth.Get(cfg.Auth)
	if p == nil {
		return cfg, nil
	}
	if cfg.Engine == "" {
		cfg.Engine = p.Engine()
	}
	defaultURL := p.DefaultServiceURL()
	if cfg.URL == "" {
		cfg.URL = defaultURL
	}
	if cfg.URL == "" && defaultURL != "" {
		return cfg, fmt.Errorf("%w: auth provider %q has no default URL", ErrURLRequired, cfg.Auth)
	}
	return cfg, nil
}

func ApplyTransportDefaults(cfg Config) Config {
	switch cfg.Transport {
	case transportVideo:
		return applyVideoDefaults(cfg)
	case transportVP8:
		return applyVP8Defaults(cfg)
	case transportSEI:
		return applySEIDefaults(cfg)
	default:
		return cfg
	}
}

func ApplyLivenessDefaults(cfg Config) Config {
	if cfg.LivenessInterval == "" {
		cfg.LivenessInterval = control.DefaultInterval.String()
	}
	if cfg.LivenessTimeout == "" {
		cfg.LivenessTimeout = control.DefaultTimeout.String()
	}
	if cfg.LivenessFailures == 0 {
		cfg.LivenessFailures = control.DefaultFailures
	}
	return cfg
}

func applyVideoDefaults(cfg Config) Config {
	if cfg.Video.Codec == "" {
		cfg.Video.Codec = videoCodecQRCode
	}
	width := defaultVideoWidth
	if cfg.Video.Codec == videoCodecTile {
		width = defaultVideoHeight
	}
	if cfg.Video.Width == 0 {
		cfg.Video.Width = width
	}
	if cfg.Video.Height == 0 {
		cfg.Video.Height = defaultVideoHeight
	}
	if cfg.Video.FPS == 0 {
		cfg.Video.FPS = defaultVideoFPS
	}
	if cfg.Video.Bitrate == "" {
		cfg.Video.Bitrate = defaultVideoBitrate
	}
	if cfg.Video.HW == "" {
		cfg.Video.HW = defaultVideoHW
	}
	if cfg.Video.QRRecovery == "" {
		cfg.Video.QRRecovery = defaultVideoQRRecovery
	}
	return cfg
}

func applyVP8Defaults(cfg Config) Config {
	if cfg.VP8.FPS == 0 {
		cfg.VP8.FPS = defaultVP8FPS
	}
	if cfg.VP8.BatchSize == 0 {
		cfg.VP8.BatchSize = defaultVP8BatchSize
	}
	return cfg
}

func applySEIDefaults(cfg Config) Config {
	if cfg.SEI.FPS == 0 {
		cfg.SEI.FPS = defaultSEIFPS
	}
	if cfg.SEI.BatchSize == 0 {
		cfg.SEI.BatchSize = defaultSEIBatchSize
	}
	if cfg.SEI.FragmentSize == 0 {
		cfg.SEI.FragmentSize = defaultSEIFragmentSize
	}
	if cfg.SEI.AckTimeoutMS == 0 {
		cfg.SEI.AckTimeoutMS = defaultSEIAckTimeoutMS
	}
	return cfg
}

func Validate(cfg Config) error {
	if err := validateMode(cfg); err != nil {
		return err
	}
	if err := validateAuth(cfg); err != nil {
		return err
	}
	if err := validateTransportRegistration(cfg); err != nil {
		return err
	}
	if err := validateCommon(cfg); err != nil {
		return err
	}
	if err := validateTransportConfig(cfg); err != nil {
		return err
	}
	if _, err := livenessConfig(cfg); err != nil {
		return err
	}
	if _, err := maxSessionDuration(cfg); err != nil {
		return err
	}
	if _, err := trafficConfig(cfg); err != nil {
		return err
	}
	return validateModeConfig(cfg)
}

func validateMode(cfg Config) error {
	switch cfg.Mode {
	case modeSRV, modeCNC, modeGen:
		return nil
	default:
		return ErrModeRequired
	}
}

func validateAuth(cfg Config) error {
	if cfg.Auth == "" {
		return ErrAuthRequired
	}
	available := enginebuiltin.Available()
	if !slices.Contains(available, cfg.Auth) {
		return fmt.Errorf("%w: %s (available: %v)", ErrUnsupportedCarrier, cfg.Auth, available)
	}
	return nil
}

func validateTransportRegistration(cfg Config) error {
	if cfg.Transport == "" {
		return ErrTransportRequired
	}
	available := transport.Available()
	if !slices.Contains(available, cfg.Transport) {
		return fmt.Errorf("%w: %s (available: %v)", ErrUnsupportedTransport, cfg.Transport, available)
	}
	return nil
}

func validateCommon(cfg Config) error {
	if cfg.RoomID == "" && cfg.Auth != authNone {
		allowAutoCreate := false
		if cfg.Mode == modeSRV {
			if p, _ := auth.Get(cfg.Auth); p != nil {
				if _, ok := p.(auth.RoomCreator); ok {
					allowAutoCreate = true
				}
			}
		}
		if !allowAutoCreate {
			return ErrRoomIDRequired
		}
	}
	// WBStream always creates a new room dynamically, bypass validation if empty
	if cfg.Auth == "wbstream" && cfg.Mode == modeSRV {
		// allow empty room ID for wbstream
	} else if cfg.RoomID == "" && cfg.Auth != authNone && cfg.Mode != modeSRV {
		// keeping original logic for other modes
	}

	if cfg.KeyHex == "" {
		return ErrKeyRequired
	}
	if cfg.DNSServer == "" {
		return ErrDNSServerRequired
	}
	return nil
}

func validateTransportConfig(cfg Config) error {
	switch cfg.Transport {
	case transportVideo:
		return validateVideoChannel(cfg)
	case transportVP8:
		return validateVP8Channel(cfg)
	case transportSEI:
		return validateSEIChannel(cfg)
	default:
		return nil
	}
}

func validateVideoCodec(cfg Config) error {
	if cfg.Video.Codec != "" && cfg.Video.Codec != videoCodecQRCode && cfg.Video.Codec != videoCodecTile {
		return ErrVideoCodecInvalid
	}
	if cfg.Video.Codec == videoCodecTile && (cfg.Video.Width != 1080 || cfg.Video.Height != 1080) {
		return ErrTileCodecDimensions
	}
	return nil
}

func validateVideoChannel(cfg Config) error {
	if cfg.Video.Width == 0 {
		return ErrVideoWidthRequired
	}
	if cfg.Video.Height == 0 {
		return ErrVideoHeightRequired
	}
	if cfg.Video.FPS == 0 {
		return ErrVideoFPSRequired
	}
	if cfg.Video.Bitrate == "" {
		return ErrVideoBitrateRequired
	}
	if cfg.Video.HW == "" {
		return ErrVideoHWRequired
	}
	return validateVideoCodec(cfg)
}

func validateVP8Channel(cfg Config) error {
	if cfg.VP8.FPS == 0 {
		return ErrVP8FPSRequired
	}
	if cfg.VP8.BatchSize == 0 {
		return ErrVP8BatchSizeRequired
	}
	return nil
}

func validateSEIChannel(cfg Config) error {
	if cfg.SEI.FPS == 0 {
		return ErrSEIFPSRequired
	}
	if cfg.SEI.BatchSize == 0 {
		return ErrSEIBatchSizeRequired
	}
	if cfg.SEI.FragmentSize == 0 {
		return ErrSEIFragmentSizeRequired
	}
	if cfg.SEI.AckTimeoutMS == 0 {
		return ErrSEIAckTimeoutRequired
	}
	return nil
}

func validateModeConfig(cfg Config) error {
	if cfg.Mode != modeCNC {
		return nil
	}
	if cfg.SOCKSHost == "" {
		return ErrSOCKSHostRequired
	}
	if cfg.SOCKSPort == 0 {
		return ErrSOCKSPortRequired
	}
	if !isLoopbackListenHost(cfg.SOCKSHost) && (cfg.SOCKSUser == "" || cfg.SOCKSPass == "") {
		return ErrSOCKSAuthRequired
	}
	return nil
}

func parseLivenessDuration(value string, def time.Duration) (time.Duration, error) {
	if value == "" {
		return def, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	if d <= 0 {
		return 0, errPositiveDuration
	}
	return d, nil
}

func livenessConfig(cfg Config) (control.Config, error) {
	interval, err := parseLivenessDuration(cfg.LivenessInterval, control.DefaultInterval)
	if err != nil {
		return control.Config{}, fmt.Errorf("%w: %w", ErrLivenessIntervalInvalid, err)
	}
	timeout, err := parseLivenessDuration(cfg.LivenessTimeout, control.DefaultTimeout)
	if err != nil {
		return control.Config{}, fmt.Errorf("%w: %w", ErrLivenessTimeoutInvalid, err)
	}
	failures := cfg.LivenessFailures
	if failures == 0 {
		failures = control.DefaultFailures
	}
	if failures < 0 {
		return control.Config{}, ErrLivenessFailuresInvalid
	}
	return control.Config{Interval: interval, Timeout: timeout, Failures: failures}, nil
}

func maxSessionDuration(cfg Config) (time.Duration, error) {
	if cfg.MaxSessionDuration == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(cfg.MaxSessionDuration)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrLifecycleMaxSessionDurationInvalid, err)
	}
	if d <= 0 {
		return 0, ErrLifecycleMaxSessionDurationInvalid
	}
	return d, nil
}

func trafficConfig(cfg Config) (transport.TrafficConfig, error) {
	if cfg.TrafficMaxPayloadSize < 0 || (cfg.TrafficMaxPayloadSize > 0 &&
		cfg.TrafficMaxPayloadSize < runtime.MinSmuxWirePayload) {
		return transport.TrafficConfig{}, ErrTrafficMaxPayloadSizeInvalid
	}
	minDelay, err := parseOptionalNonNegativeDuration(cfg.TrafficMinDelay)
	if err != nil {
		return transport.TrafficConfig{}, fmt.Errorf("%w: %w", ErrTrafficMinDelayInvalid, err)
	}
	maxDelay, err := parseOptionalNonNegativeDuration(cfg.TrafficMaxDelay)
	if err != nil {
		return transport.TrafficConfig{}, fmt.Errorf("%w: %w", ErrTrafficMaxDelayInvalid, err)
	}
	if maxDelay > 0 && maxDelay < minDelay {
		return transport.TrafficConfig{}, ErrTrafficMaxDelayInvalid
	}
	return transport.TrafficConfig{
		MaxPayloadSize: cfg.TrafficMaxPayloadSize,
		MinDelay:       minDelay,
		MaxDelay:       maxDelay,
	}, nil
}

func parseOptionalNonNegativeDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	if d < 0 {
		return 0, errNonNegativeDuration
	}
	return d, nil
}

func isLoopbackListenHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func Run(ctx context.Context, cfg Config) error {
	cfg = ApplyTransportDefaults(cfg)
	cfg = ApplyLivenessDefaults(cfg)
	configureDefaultResolver(cfg.DNSServer)
	roomURL := cfg.RoomID

	if (roomURL == "" || cfg.Auth == "wbstream") && cfg.Auth != authNone && cfg.Mode == modeSRV {
		p, _ := auth.Get(cfg.Auth)
		if creator, ok := p.(auth.RoomCreator); ok {
			var err error
			roomURL, err = creator.CreateRoom(ctx, auth.Config{
				Name:      names.Generate(),
				DNSServer: cfg.DNSServer,
				WBToken:   cfg.WBToken,
				WBCookie:  cfg.WBCookie,
			})
			if err != nil {
				return fmt.Errorf("auto create room: %w", err)
			}
			cfg.RoomID = roomURL
			logger.Infof("Auto-created room: %s", roomURL)
			if cfg.Auth == "wbstream" {
				if err := os.WriteFile("wb_stream_id", []byte(roomURL), 0644); err != nil {
					logger.Warnf("Failed to write wb_stream_id: %v", err)
				} else {
					logger.Infof("Saved WB Stream session to wb_stream_id")
				}
			}
		} else if cfg.Auth == "wbstream" {
			// This shouldn't happen, but just in case
			return fmt.Errorf("wbstream provider does not implement RoomCreator")
		}
	}

	p, _ := auth.Get(cfg.Auth)
	if keeper, ok := p.(auth.Keeper); ok {
		go keeper.KeepAlive(ctx, auth.Config{
			RoomURL:   roomURL,
			Name:      names.Generate(),
			DNSServer: cfg.DNSServer,
			WBToken:   cfg.WBToken,
			WBCookie:  cfg.WBCookie,
		})
	}

	liveness, err := livenessConfig(cfg)
	if err != nil {
		return err
	}
	maxDuration, err := maxSessionDuration(cfg)
	if err != nil {
		return err
	}
	traffic, err := trafficConfig(cfg)
	if err != nil {
		return err
	}
	run := func(ctx context.Context) error {
		return runOnce(ctx, cfg, roomURL, liveness, traffic)
	}
	if maxDuration > 0 {
		return runWithSessionRotation(ctx, maxDuration, run)
	}
	return run(ctx)
}

func configureDefaultResolver(dnsServer string) {
	if dnsServer == "" {
		return
	}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, network, dnsServer)
		},
	}
}

func runOnce(
	ctx context.Context,
	cfg Config,
	roomURL string,
	liveness control.Config,
	traffic transport.TrafficConfig,
) error {
	opts := buildTransportOptions(cfg)
	switch cfg.Mode {
	case modeSRV:
		if err := server.Run(ctx, server.Config{
			Transport:        cfg.Transport,
			Carrier:          cfg.Auth,
			RoomURL:          roomURL,
			ChannelID:        cfg.ChannelID,
			KeyHex:           cfg.KeyHex,
			DNSServer:        cfg.DNSServer,
			SOCKSProxyAddr:   cfg.SOCKSProxyAddr,
			SOCKSProxyPort:   cfg.SOCKSProxyPort,
			SOCKSProxyUser:   cfg.SOCKSProxyUser,
			SOCKSProxyPass:   cfg.SOCKSProxyPass,
			TransportOptions: opts,
			Engine:           cfg.Engine,
			URL:              cfg.URL,
			Token:            cfg.Token,
			Liveness:         liveness,
			Traffic:          traffic,
			OnSessionOpen: func(sessionID, deviceID string, claims map[string]any) {
				logger.Infof("session opened: id=%s device=%s claims=%v", sessionID, deviceID, claims)
			},
			OnSessionClose: func(sessionID, reason string) {
				logger.Infof("session closed: id=%s reason=%s", sessionID, reason)
			},
			OnTraffic: func(sessionID, addr string, bytesIn, bytesOut uint64) {
				logger.Infof("traffic: session=%s addr=%s in=%d out=%d", sessionID, addr, bytesIn, bytesOut)
			},
		}); err != nil {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	case modeCNC:
		if err := client.Run(ctx, client.Config{
			Transport:        cfg.Transport,
			Carrier:          cfg.Auth,
			RoomURL:          roomURL,
			ChannelID:        cfg.ChannelID,
			KeyHex:           cfg.KeyHex,
			LocalAddr:        fmt.Sprintf("%s:%d", cfg.SOCKSHost, cfg.SOCKSPort),
			DNSServer:        cfg.DNSServer,
			SOCKSUser:        cfg.SOCKSUser,
			SOCKSPass:        cfg.SOCKSPass,
			TransportOptions: opts,
			Engine:           cfg.Engine,
			URL:              cfg.URL,
			Token:            cfg.Token,
			Liveness:         liveness,
			Traffic:          traffic,
		}); err != nil {
			return fmt.Errorf("client: %w", err)
		}
		return nil
	default:
		return ErrModeRequired
	}
}

func runWithSessionRotation(ctx context.Context, maxDuration time.Duration, run func(context.Context) error) error {
	for cycle := 1; ; cycle++ {
		currentCycle := cycle
		runCtx, cancel := context.WithCancel(ctx)
		var rotated atomic.Bool
		timer := time.AfterFunc(maxDuration, func() {
			rotated.Store(true)
			logger.Infof("session max duration reached: duration=%s cycle=%d", maxDuration, currentCycle)
			cancel()
		})
		err := run(runCtx)
		cancel()
		timer.Stop()
		if ctx.Err() != nil {
			return nil
		}
		if rotated.Load() {
			if err != nil {
				logger.Warnf("session rotation ended with error: cycle=%d err=%v", currentCycle, err)
			}
			logger.Infof("session rotation restarting: next_cycle=%d", currentCycle+1)
			if err := waitSessionRestart(ctx); err != nil {
				return nil
			}
			continue
		}
		if err != nil {
			return err
		}
		logger.Infof("session ended cleanly with lifecycle rotation enabled: next_cycle=%d", currentCycle+1)
		if err := waitSessionRestart(ctx); err != nil {
			return nil
		}
	}
}

func waitSessionRestart(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("restart delay canceled: %w", ctx.Err())
	case <-time.After(sessionRestartDelay):
		return nil
	}
}

func ValidateGen(cfg Config) error {
	if cfg.Auth == "" {
		return ErrAuthRequired
	}
	available := enginebuiltin.Available()
	if !slices.Contains(available, cfg.Auth) {
		return fmt.Errorf("%w: %s (available: %v)", ErrUnsupportedCarrier, cfg.Auth, available)
	}
	if cfg.DNSServer == "" {
		return ErrDNSServerRequired
	}
	if cfg.Amount < 1 {
		return ErrAmountRequired
	}
	p, err := auth.Get(cfg.Auth)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedCarrier, cfg.Auth)
	}
	if _, ok := p.(auth.RoomCreator); !ok {
		return fmt.Errorf("%w: %s does not support room generation", ErrUnsupportedCarrier, cfg.Auth)
	}
	return nil
}

const (
	genMaxAttempts = 5
	genRetryDelay  = 2 * time.Second
)

func genRetry(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	for attempt := range genMaxAttempts {
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if attempt < genMaxAttempts-1 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("context canceled: %w", ctx.Err())
			case <-time.After(genRetryDelay):
			}
		}
	}
	return lastErr
}

func Gen(ctx context.Context, cfg Config, out func(string)) error {
	configureDefaultResolver(cfg.DNSServer)
	p, err := auth.Get(cfg.Auth)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedCarrier, cfg.Auth)
	}
	creator, ok := p.(auth.RoomCreator)
	if !ok {
		return fmt.Errorf("%w: %s does not support room generation", ErrUnsupportedCarrier, cfg.Auth)
	}
	for i := range cfg.Amount {
		var roomID string
		err := genRetry(ctx, func(ctx context.Context) error {
			var genErr error
			roomID, genErr = creator.CreateRoom(ctx, auth.Config{
				Name:      names.Generate(),
				DNSServer: cfg.DNSServer,
				WBToken:   cfg.WBToken,
				WBCookie:  cfg.WBCookie,
			})
			if genErr != nil {
				return fmt.Errorf("CreateRoom: %w", genErr)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("gen room %d: %w", i+1, err)
		}
		out(roomID)
	}
	return nil
}
