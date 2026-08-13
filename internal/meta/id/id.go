package id

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	timeWidth      = 13
	generatorWidth = 10
	sequenceWidth  = 4
	entropyWidth   = 6
	maxSequence    = 36*36*36*36 - 1
)

const (
	base36Alphabet    = "0123456789abcdefghijklmnopqrstuvwxyz"
	crockfordAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"
)

type Kind string

const (
	KindBucket  Kind = "bkt"
	KindVersion Kind = "ver"
	KindUpload  Kind = "upl"
)

type Generator interface {
	NewID(kind Kind) (string, error)
}

type GeneratorFunc func(kind Kind) (string, error)

func (f GeneratorFunc) NewID(kind Kind) (string, error) {
	return f(kind)
}

type Option func(*GeneratorConfig)

type GeneratorConfig struct {
	Now         func() time.Time
	Entropy     io.Reader
	GeneratorID string
}

type ProcessGenerator struct {
	mu           sync.Mutex
	now          func() time.Time
	entropy      io.Reader
	generatorID  string
	lastUnixNano uint64
	sequence     uint64
}

func NewProcessGenerator(options ...Option) (*ProcessGenerator, error) {
	cfg := GeneratorConfig{
		Now:     func() time.Time { return time.Now().UTC() },
		Entropy: rand.Reader,
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if cfg.Now == nil {
		return nil, errors.New("id clock is required")
	}
	if cfg.Entropy == nil {
		return nil, errors.New("id entropy reader is required")
	}
	generatorID := strings.TrimSpace(strings.ToLower(cfg.GeneratorID))
	if generatorID == "" {
		var err error
		generatorID, err = randomCrockford(cfg.Entropy, generatorWidth)
		if err != nil {
			return nil, fmt.Errorf("generate id generator id: %w", err)
		}
	}
	if !validToken(generatorID, generatorWidth, crockfordAlphabet) {
		return nil, fmt.Errorf("invalid id generator id %q", cfg.GeneratorID)
	}
	return &ProcessGenerator{
		now:         cfg.Now,
		entropy:     cfg.Entropy,
		generatorID: generatorID,
	}, nil
}

func MustNewProcessGenerator(options ...Option) *ProcessGenerator {
	generator, err := NewProcessGenerator(options...)
	if err != nil {
		panic(err)
	}
	return generator
}

func WithClock(now func() time.Time) Option {
	return func(cfg *GeneratorConfig) {
		cfg.Now = now
	}
}

func WithEntropy(reader io.Reader) Option {
	return func(cfg *GeneratorConfig) {
		cfg.Entropy = reader
	}
}

func WithGeneratorID(generatorID string) Option {
	return func(cfg *GeneratorConfig) {
		cfg.GeneratorID = generatorID
	}
}

func (g *ProcessGenerator) NewID(kind Kind) (string, error) {
	if !kind.Valid() {
		return "", fmt.Errorf("invalid id kind %q", kind)
	}
	g.mu.Lock()
	timestamp, sequence, err := g.nextTimestampAndSequenceLocked()
	if err != nil {
		g.mu.Unlock()
		return "", err
	}
	entropy, err := randomCrockford(g.entropy, entropyWidth)
	g.mu.Unlock()
	if err != nil {
		return "", fmt.Errorf("generate id entropy: %w", err)
	}
	return fmt.Sprintf("%s_%s_%s_%s_%s",
		kind,
		formatBase36(timestamp, timeWidth),
		g.generatorID,
		formatBase36(sequence, sequenceWidth),
		entropy,
	), nil
}

func (g *ProcessGenerator) nextTimestampAndSequenceLocked() (uint64, uint64, error) {
	for {
		now := g.now().UnixNano()
		if now < 0 {
			return 0, 0, errors.New("id clock before unix epoch")
		}
		timestamp := uint64(now)
		switch {
		case timestamp > g.lastUnixNano:
			g.lastUnixNano = timestamp
			g.sequence = 0
			return timestamp, 0, nil
		case timestamp == g.lastUnixNano && g.sequence < maxSequence:
			g.sequence++
			return timestamp, g.sequence, nil
		default:
			g.mu.Unlock()
			time.Sleep(time.Nanosecond)
			g.mu.Lock()
		}
	}
}

func (k Kind) Valid() bool {
	switch k {
	case KindBucket, KindVersion, KindUpload:
		return true
	default:
		return false
	}
}

type DeterministicGenerator struct {
	mu     sync.Mutex
	values map[Kind][]string
	next   map[Kind]int
}

func NewDeterministicGenerator(values map[Kind][]string) *DeterministicGenerator {
	copied := make(map[Kind][]string, len(values))
	for kind, ids := range values {
		copied[kind] = append([]string(nil), ids...)
	}
	return &DeterministicGenerator{
		values: copied,
		next:   make(map[Kind]int),
	}
}

func (g *DeterministicGenerator) NewID(kind Kind) (string, error) {
	if !kind.Valid() {
		return "", fmt.Errorf("invalid id kind %q", kind)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	index := g.next[kind]
	ids := g.values[kind]
	if index >= len(ids) {
		return "", fmt.Errorf("deterministic id generator exhausted for kind %q", kind)
	}
	g.next[kind] = index + 1
	return ids[index], nil
}

func randomCrockford(reader io.Reader, width int) (string, error) {
	buf := make([]byte, width)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", err
	}
	out := make([]byte, width)
	for i, value := range buf {
		out[i] = crockfordAlphabet[int(value)&31]
	}
	return string(out), nil
}

func formatBase36(value uint64, width int) string {
	out := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		out[i] = base36Alphabet[value%36]
		value /= 36
	}
	return string(out)
}

func validToken(value string, width int, alphabet string) bool {
	if len(value) != width {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune(alphabet, char) {
			return false
		}
	}
	return true
}
