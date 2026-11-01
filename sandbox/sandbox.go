package sandbox

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"sync"

	"github.com/hashicorp/hcl/v2"
)

type NetMode int

const (
	NetNone NetMode = iota
	NetAll
	NetAllowList
)

type Network struct {
	Mode  NetMode
	Hosts []string
}

type Policy struct {
	Workdir   string
	ReadOnly  []string
	ReadWrite []string
	Deny      []string
	Network   Network
	Env       []string
}

type Cmd struct {
	Args []string
	Dir  string
	Env  map[string]string
}

// Result.ExitCode is -1 if the command was killed.
type Result struct {
	Output   []byte
	ExitCode int
}

type Sandbox interface {
	// Run returns an error only if the command could not be run; a non-zero exit is
	// reported in the Result.
	Run(ctx context.Context, cmd Cmd) (Result, error)
	FS() FS
	Close(ctx context.Context) error
}

type FS interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
	MkdirAll(ctx context.Context, path string, perm fs.FileMode) error
	Remove(ctx context.Context, path string) error
	Stat(ctx context.Context, path string) (fs.FileInfo, error)
	WalkDir(ctx context.Context, root string, fn fs.WalkDirFunc) error
}

type Backend interface {
	Name() string
	Available() error
	New(ctx context.Context, p Policy, cfg hcl.Body) (Sandbox, error)
}

var (
	backendsMu sync.Mutex
	backends   = map[string]Backend{}
)

func Register(b Backend) {
	backendsMu.Lock()
	defer backendsMu.Unlock()

	if _, ok := backends[b.Name()]; ok {
		panic(fmt.Sprintf("sandbox: backend registered twice: %s", b.Name()))
	}
	backends[b.Name()] = b
}

func Backends() []string {
	backendsMu.Lock()
	defer backendsMu.Unlock()

	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Lookup(name string) (Backend, bool) {
	backendsMu.Lock()
	defer backendsMu.Unlock()

	b, ok := backends[name]
	return b, ok
}

func Open(ctx context.Context, name string, p Policy, cfg hcl.Body) (Sandbox, error) {
	b, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("sandbox: unknown backend: %s (available: %v)", name,
			Backends())
	}
	if err := b.Available(); err != nil {
		return nil, fmt.Errorf("sandbox: %s: %w", name, err)
	}

	return b.New(ctx, p, cfg)
}
