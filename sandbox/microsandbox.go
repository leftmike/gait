//go:build microsandbox

package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func init() {
	Register(microsandboxBackend{})
}

type microsandboxConfig struct {
	Image  string `hcl:"image"`
	Memory uint32 `hcl:"memory,optional"`
	CPUs   uint8  `hcl:"cpus,optional"`
}

type microsandboxBackend struct{}

func (microsandboxBackend) Name() string {
	return "microsandbox"
}

func (microsandboxBackend) Available() error {
	if runtime.GOOS == "linux" {
		f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
		if err != nil {
			return err
		}
		f.Close()
	}
	return nil
}

type microsandbox struct {
	name   string
	policy Policy
	hfs    *HostFS
	opts   []msb.SandboxOption
	tmpdir string

	mu sync.Mutex
	sb *msb.Sandbox
}

func (microsandboxBackend) New(ctx context.Context, p Policy, cfg hcl.Body) (Sandbox, error) {
	if cfg == nil {
		return nil, errors.New("microsandbox: a microsandbox block with an image is required")
	}
	var mc microsandboxConfig
	if diags := gohcl.DecodeBody(cfg, nil, &mc); diags.HasErrors() {
		return nil, diags
	}

	hfs, err := NewHostFS(p)
	if err != nil {
		return nil, err
	}

	var suffix [4]byte
	rand.Read(suffix[:])
	tmpdir, err := os.MkdirTemp("", "gait-microsandbox-")
	if err != nil {
		return nil, err
	}

	sb := &microsandbox{
		name:   fmt.Sprintf("gait-%d-%s", os.Getpid(), hex.EncodeToString(suffix[:])),
		policy: p,
		hfs:    hfs,
		tmpdir: tmpdir,
	}

	mounts, err := sb.mounts()
	if err != nil {
		os.RemoveAll(tmpdir)
		return nil, err
	}

	env := map[string]string{}
	for _, name := range p.Env {
		if val, ok := os.LookupEnv(name); ok {
			env[name] = val
		}
	}

	sb.opts = []msb.SandboxOption{
		msb.WithImage(mc.Image),
		msb.WithWorkdir(p.Workdir),
		msb.WithMounts(mounts),
		msb.WithNetwork(networkConfig(p.Network)),
		msb.WithEnv(env),
		msb.WithEphemeral(true),
		msb.WithQuietLogs(),
	}
	if mc.Memory > 0 {
		sb.opts = append(sb.opts, msb.WithMemory(mc.Memory))
	}
	if mc.CPUs > 0 {
		sb.opts = append(sb.opts, msb.WithCPUs(mc.CPUs))
	}

	return sb, nil
}

// mounts places each policy path at the same path in the guest. A denied directory is
// covered by an empty read-only tmpfs, and a denied file by an empty read-only file.
func (sb *microsandbox) mounts() (map[string]msb.MountConfig, error) {
	mounts := map[string]msb.MountConfig{}
	for _, r := range sb.hfs.rules {
		if r.path == "/" {
			return nil, errors.New("microsandbox: / cannot be mounted")
		}

		fi, err := os.Stat(r.path)
		if errors.Is(err, fs.ErrNotExist) {
			if r.acc == noAccess {
				mounts[r.path] = msb.Mount.Tmpfs(msb.TmpfsOptions{SizeMiB: 1, Readonly: true})
			}
			continue
		} else if err != nil {
			return nil, err
		}

		switch r.acc {
		case writeAccess, readAccess:
			mounts[r.path] = msb.Mount.Bind(r.path, msb.MountOptions{
				Readonly:        r.acc == readAccess,
				HostPermissions: msb.HostPermissionsMirror,
			})
		case noAccess:
			if fi.IsDir() {
				mounts[r.path] = msb.Mount.Tmpfs(msb.TmpfsOptions{SizeMiB: 1, Readonly: true})
			} else {
				empty := filepath.Join(sb.tmpdir, fmt.Sprintf("deny-%d", len(mounts)))
				if err := os.WriteFile(empty, nil, 0o400); err != nil {
					return nil, err
				}
				mounts[r.path] = msb.Mount.Bind(empty, msb.MountOptions{Readonly: true})
			}
		}
	}
	return mounts, nil
}

func networkConfig(n Network) *msb.NetworkConfig {
	nc := &msb.NetworkConfig{DefaultEgress: msb.PolicyActionDeny}
	switch n.Mode {
	case NetAll:
		nc.DefaultEgress = msb.PolicyActionAllow
	case NetAllowList:
		nc.Rules = append(nc.Rules, msb.Rule.AllowDNS())
		for _, host := range n.Hosts {
			nc.Rules = append(nc.Rules, msb.PolicyRule{
				Action:      msb.PolicyActionAllow,
				Direction:   msb.PolicyDirectionEgress,
				Destination: host,
			})
		}
	}
	return nc
}

// start creates the VM if it is not running. A command's deadline does not apply: an
// interrupted creation would leave a partial sandbox behind.
func (sb *microsandbox) start(ctx context.Context) (*msb.Sandbox, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if sb.sb != nil {
		return sb.sb, nil
	}

	ctx = context.WithoutCancel(ctx)
	_, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{})
	if err == nil {
		sb.sb, err = msb.CreateSandbox(ctx, sb.name, sb.opts...)
	}
	if err != nil {
		msb.RemoveSandbox(ctx, sb.name)
		return nil, fmt.Errorf("microsandbox: %w", err)
	}
	return sb.sb, nil
}

func (sb *microsandbox) Run(ctx context.Context, cmd Cmd) (Result, error) {
	if len(cmd.Args) == 0 {
		return Result{}, errors.New("sandbox: empty command")
	}

	dir, err := sb.hfs.Dir(cmd.Dir)
	if err != nil {
		return Result{}, err
	}
	if err := sb.hfs.checkPath("chdir", dir, readAccess); err != nil {
		return Result{}, err
	}

	vm, err := sb.start(ctx)
	if err != nil {
		return Result{}, err
	} else if ctx.Err() != nil {
		return Result{ExitCode: -1}, nil
	}

	opts := []msb.ExecOption{msb.WithExecCwd(dir)}
	if len(cmd.Env) > 0 {
		opts = append(opts, msb.WithExecEnv(cmd.Env))
	}
	h, err := vm.ExecStream(ctx, cmd.Args[0], cmd.Args[1:], opts...)
	if err != nil {
		return Result{}, err
	}
	defer h.Close()

	var out []byte
	for {
		ev, err := h.Recv(ctx)
		if err != nil {
			if ctx.Err() != nil {
				kctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				h.Kill(kctx)
				cancel()
				return Result{Output: out, ExitCode: -1}, nil
			}
			return Result{Output: out}, err
		}

		switch ev.Kind {
		case msb.ExecEventStdout, msb.ExecEventStderr:
			out = append(out, ev.Data...)
		case msb.ExecEventExited:
			return Result{Output: out, ExitCode: ev.ExitCode}, nil
		case msb.ExecEventFailed:
			if ev.Failure != nil {
				return Result{Output: out}, fmt.Errorf("microsandbox: %s: %v", cmd.Args[0],
					ev.Failure)
			}
			return Result{Output: out}, fmt.Errorf("microsandbox: %s: failed to start",
				cmd.Args[0])
		case msb.ExecEventDone:
			return Result{Output: out}, errors.New("microsandbox: no exit status")
		}
	}
}

func (sb *microsandbox) FS() FS {
	return sb.hfs
}

func (sb *microsandbox) Close(ctx context.Context) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	var errs []error
	if sb.sb != nil {
		errs = append(errs, sb.sb.Stop(ctx), sb.sb.Close())
		// An ephemeral sandbox is usually removed when it stops.
		if err := msb.RemoveSandbox(ctx, sb.name); !msb.IsKind(err, msb.ErrSandboxNotFound) {
			errs = append(errs, err)
		}
		sb.sb = nil
	}
	errs = append(errs, os.RemoveAll(sb.tmpdir))
	return errors.Join(errs...)
}
