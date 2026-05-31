package names

import (
	"bufio"
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

//go:embed data/names
var embeddedNames string

//go:embed data/surnames
var embeddedSurnames string

var (
	firstNames = parseEmbedded(embeddedNames)
	lastNames  = parseEmbedded(embeddedSurnames)
)

func parseEmbedded(raw string) []string {
	names := make([]string, 0, strings.Count(raw, "\n")+1)
	for len(raw) > 0 {
		i := strings.IndexByte(raw, '\n')
		var line string
		if i < 0 {
			line, raw = raw, ""
		} else {
			line, raw = raw[:i], raw[i+1:]
		}
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

func loadNames(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open names file %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()
	names := make([]string, 0, 256)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			names = append(names, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan names file %q: %w", path, err)
	}
	return names, nil
}

func LoadNameFiles(firstPath, lastPath string) error {
	if names, err := loadNames(firstPath); err == nil && len(names) > 0 {
		firstNames = names
	}
	if names, err := loadNames(lastPath); err == nil && len(names) > 0 {
		lastNames = names
	}
	return nil
}

func Generate() string {
	first, last := firstNames, lastNames
	if len(first) == 0 || len(last) == 0 {
		return "anonymous user"
	}
	fn := first[randomIndex(len(first))]
	ln := last[randomIndex(len(last))]
	var b strings.Builder
	b.Grow(len(fn) + 1 + len(ln))
	b.WriteString(fn)
	b.WriteByte(' ')
	b.WriteString(ln)
	return b.String()
}

func randomIndex(limit int) int {
	if limit <= 1 {
		return 0
	}
	max := uint64(limit)
	threshold := (0 - max) % max
	var buf [8]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0
		}
		v := binary.BigEndian.Uint64(buf[:])
		if v >= threshold {
			return int(v % max)
		}
	}
}
