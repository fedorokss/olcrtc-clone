package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/fedorokss/olcrtc-clone/internal/app/session"
	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound     = errors.New("config file not found")
	ErrConfigInvalidUTF8  = errors.New("config file is not valid UTF-8")
	ErrCryptoKeyConflict  = errors.New("crypto.key and crypto.key_file cannot both be set")
	ErrCryptoKeyFileEmpty = errors.New("crypto key file is empty")
)

type File struct {
	Mode      string    `yaml:"mode"`
	Auth      Auth      `yaml:"auth"`
	Room      Room      `yaml:"room"`
	Crypto    Crypto    `yaml:"crypto"`
	Net       Net       `yaml:"net"`
	SOCKS     SOCKS     `yaml:"socks"`
	Engine    Engine    `yaml:"engine"`
	Video     Video     `yaml:"video"`
	VP8       VP8       `yaml:"vp8"`
	SEI       SEI       `yaml:"sei"`
	Liveness  Liveness  `yaml:"liveness"`
	Lifecycle Lifecycle `yaml:"lifecycle"`
	Traffic   Traffic   `yaml:"traffic"`
	Gen       Gen       `yaml:"gen"`
	Profiles  []Profile `yaml:"profiles"`
	Failover  Failover  `yaml:"failover"`
	Data      string    `yaml:"data"`
	Debug     bool      `yaml:"debug"`
}

type Profile struct {
	Name      string    `yaml:"name"`
	Auth      Auth      `yaml:"auth"`
	Room      Room      `yaml:"room"`
	Crypto    Crypto    `yaml:"crypto"`
	Net       Net       `yaml:"net"`
	SOCKS     SOCKS     `yaml:"socks"`
	Engine    Engine    `yaml:"engine"`
	Video     Video     `yaml:"video"`
	VP8       VP8       `yaml:"vp8"`
	SEI       SEI       `yaml:"sei"`
	Liveness  Liveness  `yaml:"liveness"`
	Lifecycle Lifecycle `yaml:"lifecycle"`
	Traffic   Traffic   `yaml:"traffic"`
}

type Failover struct {
	RetryDelay string `yaml:"retry_delay"`
	MaxCycles  int    `yaml:"max_cycles"`
}

type Auth struct {
	Provider string `yaml:"provider"`
	WBToken  string `yaml:"wb_token"`
	WBCookie string `yaml:"wb_cookie"`
}

type Room struct {
	ID      string `yaml:"id"`
	Channel string `yaml:"channel"`
}

type Crypto struct {
	Key     string `yaml:"key"`
	KeyFile string `yaml:"key_file"`
}

type Net struct {
	Transport string `yaml:"transport"`
	DNS       string `yaml:"dns"`
}

type SOCKS struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	User      string `yaml:"user"`
	Pass      string `yaml:"pass"`
	ProxyAddr string `yaml:"proxy_addr"`
	ProxyPort int    `yaml:"proxy_port"`
	ProxyUser string `yaml:"proxy_user"`
	ProxyPass string `yaml:"proxy_pass"`
}

type Engine struct {
	Name  string `yaml:"name"`
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type Video struct {
	Width      int    `yaml:"width"`
	Height     int    `yaml:"height"`
	FPS        int    `yaml:"fps"`
	Bitrate    string `yaml:"bitrate"`
	HW         string `yaml:"hw"`
	QRSize     int    `yaml:"qr_size"`
	QRRecovery string `yaml:"qr_recovery"`
	Codec      string `yaml:"codec"`
	TileModule int    `yaml:"tile_module"`
	TileRS     int    `yaml:"tile_rs"`
}

type VP8 struct {
	FPS       int `yaml:"fps"`
	BatchSize int `yaml:"batch_size"`
}

type SEI struct {
	FPS          int `yaml:"fps"`
	BatchSize    int `yaml:"batch_size"`
	FragmentSize int `yaml:"fragment_size"`
	AckTimeoutMS int `yaml:"ack_timeout_ms"`
}

type Liveness struct {
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
	Failures int    `yaml:"failures"`
}

type Lifecycle struct {
	MaxSessionDuration string `yaml:"max_session_duration"`
}

type Traffic struct {
	MaxPayloadSize int    `yaml:"max_payload_size"`
	MinDelay       string `yaml:"min_delay"`
	MaxDelay       string `yaml:"max_delay"`
}

type Gen struct {
	Amount int `yaml:"amount"`
}

func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, fmt.Errorf("%w: %s", ErrConfigNotFound, path)
		}
		return File{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if !utf8.Valid(data) {
		return File{}, fmt.Errorf("parse config %s: %w", path, ErrConfigInvalidUTF8)
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := loadExternalSecrets(path, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

func loadExternalSecrets(configPath string, f *File) error {
	baseDir := filepath.Dir(configPath)
	if f.Crypto.KeyFile != "" {
		if f.Crypto.Key != "" {
			return ErrCryptoKeyConflict
		}
		key, err := readKeyFile(baseDir, f.Crypto.KeyFile)
		if err != nil {
			return err
		}
		f.Crypto.Key = key
	}
	return loadProfileSecrets(baseDir, f.Profiles)
}

func loadProfileSecrets(baseDir string, profiles []Profile) error {
	for i := range profiles {
		p := &profiles[i]
		if p.Crypto.KeyFile == "" {
			continue
		}
		if p.Crypto.Key != "" {
			return fmt.Errorf("profiles[%d]: %w", i, ErrCryptoKeyConflict)
		}
		key, err := readKeyFile(baseDir, p.Crypto.KeyFile)
		if err != nil {
			return fmt.Errorf("profiles[%d]: %w", i, err)
		}
		p.Crypto.Key = key
	}
	return nil
}

func readKeyFile(baseDir, keyFile string) (string, error) {
	keyPath := keyFile
	if !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(baseDir, keyPath)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("read crypto key file %s: %w", keyPath, err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", ErrCryptoKeyFileEmpty
	}
	return key, nil
}

func Apply(dst session.Config, f File) session.Config {
	dst.Mode = coalesce(dst.Mode, f.Mode)
	dst.Transport = coalesce(dst.Transport, f.Net.Transport)
	dst.Auth = coalesce(dst.Auth, f.Auth.Provider)
	dst.WBToken = coalesce(dst.WBToken, f.Auth.WBToken)
	dst.WBCookie = coalesce(dst.WBCookie, f.Auth.WBCookie)
	dst.Engine = coalesce(dst.Engine, f.Engine.Name)
	dst.URL = coalesce(dst.URL, f.Engine.URL)
	dst.Token = coalesce(dst.Token, f.Engine.Token)
	dst.RoomID = coalesce(dst.RoomID, f.Room.ID)
	dst.ChannelID = coalesce(dst.ChannelID, f.Room.Channel)
	dst.KeyHex = coalesce(dst.KeyHex, f.Crypto.Key)
	dst.SOCKSHost = coalesce(dst.SOCKSHost, f.SOCKS.Host)
	dst.SOCKSPort = coalesce(dst.SOCKSPort, f.SOCKS.Port)
	dst.SOCKSUser = coalesce(dst.SOCKSUser, f.SOCKS.User)
	dst.SOCKSPass = coalesce(dst.SOCKSPass, f.SOCKS.Pass)
	dst.DNSServer = coalesce(dst.DNSServer, f.Net.DNS)
	dst.SOCKSProxyAddr = coalesce(dst.SOCKSProxyAddr, f.SOCKS.ProxyAddr)
	dst.SOCKSProxyPort = coalesce(dst.SOCKSProxyPort, f.SOCKS.ProxyPort)
	dst.SOCKSProxyUser = coalesce(dst.SOCKSProxyUser, f.SOCKS.ProxyUser)
	dst.SOCKSProxyPass = coalesce(dst.SOCKSProxyPass, f.SOCKS.ProxyPass)
	dst.Video.Width = coalesce(dst.Video.Width, f.Video.Width)
	dst.Video.Height = coalesce(dst.Video.Height, f.Video.Height)
	dst.Video.FPS = coalesce(dst.Video.FPS, f.Video.FPS)
	dst.Video.Bitrate = coalesce(dst.Video.Bitrate, f.Video.Bitrate)
	dst.Video.HW = coalesce(dst.Video.HW, f.Video.HW)
	dst.Video.QRSize = coalesce(dst.Video.QRSize, f.Video.QRSize)
	dst.Video.QRRecovery = coalesce(dst.Video.QRRecovery, f.Video.QRRecovery)
	dst.Video.Codec = coalesce(dst.Video.Codec, f.Video.Codec)
	dst.Video.TileModule = coalesce(dst.Video.TileModule, f.Video.TileModule)
	dst.Video.TileRS = coalesce(dst.Video.TileRS, f.Video.TileRS)
	dst.VP8.FPS = coalesce(dst.VP8.FPS, f.VP8.FPS)
	dst.VP8.BatchSize = coalesce(dst.VP8.BatchSize, f.VP8.BatchSize)
	dst.SEI.FPS = coalesce(dst.SEI.FPS, f.SEI.FPS)
	dst.SEI.BatchSize = coalesce(dst.SEI.BatchSize, f.SEI.BatchSize)
	dst.SEI.FragmentSize = coalesce(dst.SEI.FragmentSize, f.SEI.FragmentSize)
	dst.SEI.AckTimeoutMS = coalesce(dst.SEI.AckTimeoutMS, f.SEI.AckTimeoutMS)
	dst.LivenessInterval = coalesce(dst.LivenessInterval, f.Liveness.Interval)
	dst.LivenessTimeout = coalesce(dst.LivenessTimeout, f.Liveness.Timeout)
	dst.LivenessFailures = coalesce(dst.LivenessFailures, f.Liveness.Failures)
	dst.MaxSessionDuration = coalesce(dst.MaxSessionDuration, f.Lifecycle.MaxSessionDuration)
	dst.TrafficMaxPayloadSize = coalesce(dst.TrafficMaxPayloadSize, f.Traffic.MaxPayloadSize)
	dst.TrafficMinDelay = coalesce(dst.TrafficMinDelay, f.Traffic.MinDelay)
	dst.TrafficMaxDelay = coalesce(dst.TrafficMaxDelay, f.Traffic.MaxDelay)
	dst.Amount = coalesce(dst.Amount, f.Gen.Amount)
	return dst
}

func ApplyProfile(base session.Config, p Profile) session.Config {
	dst := base
	dst.Transport = coalesce(p.Net.Transport, dst.Transport)
	dst.Auth = coalesce(p.Auth.Provider, dst.Auth)
	dst.WBToken = coalesce(p.Auth.WBToken, dst.WBToken)
	dst.WBCookie = coalesce(p.Auth.WBCookie, dst.WBCookie)
	dst.Engine = coalesce(p.Engine.Name, dst.Engine)
	dst.URL = coalesce(p.Engine.URL, dst.URL)
	dst.Token = coalesce(p.Engine.Token, dst.Token)
	dst.RoomID = coalesce(p.Room.ID, dst.RoomID)
	dst.ChannelID = coalesce(p.Room.Channel, dst.ChannelID)
	dst.KeyHex = coalesce(p.Crypto.Key, dst.KeyHex)
	dst.SOCKSHost = coalesce(p.SOCKS.Host, dst.SOCKSHost)
	dst.SOCKSPort = coalesce(p.SOCKS.Port, dst.SOCKSPort)
	dst.SOCKSUser = coalesce(p.SOCKS.User, dst.SOCKSUser)
	dst.SOCKSPass = coalesce(p.SOCKS.Pass, dst.SOCKSPass)
	dst.DNSServer = coalesce(p.Net.DNS, dst.DNSServer)
	dst.SOCKSProxyAddr = coalesce(p.SOCKS.ProxyAddr, dst.SOCKSProxyAddr)
	dst.SOCKSProxyPort = coalesce(p.SOCKS.ProxyPort, dst.SOCKSProxyPort)
	dst.SOCKSProxyUser = coalesce(p.SOCKS.ProxyUser, dst.SOCKSProxyUser)
	dst.SOCKSProxyPass = coalesce(p.SOCKS.ProxyPass, dst.SOCKSProxyPass)
	dst.Video.Width = coalesce(p.Video.Width, dst.Video.Width)
	dst.Video.Height = coalesce(p.Video.Height, dst.Video.Height)
	dst.Video.FPS = coalesce(p.Video.FPS, dst.Video.FPS)
	dst.Video.Bitrate = coalesce(p.Video.Bitrate, dst.Video.Bitrate)
	dst.Video.HW = coalesce(p.Video.HW, dst.Video.HW)
	dst.Video.QRSize = coalesce(p.Video.QRSize, dst.Video.QRSize)
	dst.Video.QRRecovery = coalesce(p.Video.QRRecovery, dst.Video.QRRecovery)
	dst.Video.Codec = coalesce(p.Video.Codec, dst.Video.Codec)
	dst.Video.TileModule = coalesce(p.Video.TileModule, dst.Video.TileModule)
	dst.Video.TileRS = coalesce(p.Video.TileRS, dst.Video.TileRS)
	dst.VP8.FPS = coalesce(p.VP8.FPS, dst.VP8.FPS)
	dst.VP8.BatchSize = coalesce(p.VP8.BatchSize, dst.VP8.BatchSize)
	dst.SEI.FPS = coalesce(p.SEI.FPS, dst.SEI.FPS)
	dst.SEI.BatchSize = coalesce(p.SEI.BatchSize, dst.SEI.BatchSize)
	dst.SEI.FragmentSize = coalesce(p.SEI.FragmentSize, dst.SEI.FragmentSize)
	dst.SEI.AckTimeoutMS = coalesce(p.SEI.AckTimeoutMS, dst.SEI.AckTimeoutMS)
	dst.LivenessInterval = coalesce(p.Liveness.Interval, dst.LivenessInterval)
	dst.LivenessTimeout = coalesce(p.Liveness.Timeout, dst.LivenessTimeout)
	dst.LivenessFailures = coalesce(p.Liveness.Failures, dst.LivenessFailures)
	dst.MaxSessionDuration = coalesce(p.Lifecycle.MaxSessionDuration, dst.MaxSessionDuration)
	dst.TrafficMaxPayloadSize = coalesce(p.Traffic.MaxPayloadSize, dst.TrafficMaxPayloadSize)
	dst.TrafficMinDelay = coalesce(p.Traffic.MinDelay, dst.TrafficMinDelay)
	dst.TrafficMaxDelay = coalesce(p.Traffic.MaxDelay, dst.TrafficMaxDelay)
	return dst
}

func coalesce[T comparable](primary, fallback T) T {
	var zero T
	if primary != zero {
		return primary
	}
	return fallback
}
