package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

func init() {
	Register(nonoBackend{})
}

type nonoConfig struct {
	Binary string `hcl:"binary,optional"`
}

type nonoBackend struct{}

func (nonoBackend) Name() string {
	return "nono"
}

func (nonoBackend) Available() error {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return fmt.Errorf("not supported on %s", runtime.GOOS)
	}
	return nil
}

type nono struct {
	bin     string
	policy  Policy
	hfs     *HostFS
	tmpdir  string
	exclude []string
	devices []string
}

func (nonoBackend) New(ctx context.Context, p Policy, cfg hcl.Body) (Sandbox, error) {
	var nc nonoConfig
	if cfg != nil {
		if diags := gohcl.DecodeBody(cfg, nil, &nc); diags.HasErrors() {
			return nil, diags
		}
	}
	if nc.Binary == "" {
		nc.Binary = "nono"
	}
	bin, err := exec.LookPath(nc.Binary)
	if err != nil {
		return nil, err
	}

	hfs, err := NewHostFS(p)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "linux" {
		if err := landlockConflicts(hfs.rules); err != nil {
			return nil, err
		}
	}

	tmpdir, err := os.MkdirTemp("", "gait-nono-")
	if err != nil {
		return nil, err
	}

	sb := &nono{
		bin:    bin,
		policy: p,
		hfs:    hfs,
		tmpdir: tmpdir,
	}

	// The default groups are excluded where they would grant more than the policy:
	// writes to /tmp, extra read paths, and refusing to start rm, mv, cp and others.
	switch runtime.GOOS {
	case "linux":
		sb.exclude = []string{"system_write_linux", "user_tools", "homebrew_linux",
			"dangerous_commands", "dangerous_commands_linux"}
		sb.devices = []string{"/dev/null", "/dev/zero", "/dev/full", "/dev/tty",
			"/dev/stdout", "/dev/stderr", "/dev/fd", "/dev/pts", "/proc/self/fd"}
	case "darwin":
		sb.exclude = []string{"system_write_macos", "user_tools", "homebrew_macos",
			"dangerous_commands", "dangerous_commands_macos"}
		sb.devices = []string{"/dev"}
	}

	res, err := sb.Run(ctx, Cmd{Args: []string{"true"}})
	if err != nil {
		os.RemoveAll(tmpdir)
		return nil, fmt.Errorf("nono: %w", err)
	} else if res.ExitCode != 0 {
		os.RemoveAll(tmpdir)
		return nil, errors.New(strings.TrimSpace(string(res.Output)))
	}
	return sb, nil
}

// landlockConflicts rejects a path given less access than a directory containing it:
// Landlock only grants access, so the more restrictive entry would have no effect.
func landlockConflicts(rules []rule) error {
	for _, inner := range rules {
		for _, outer := range rules {
			if inner.path != outer.path && within(inner.path, outer.path) &&
				inner.acc < outer.acc {

				return fmt.Errorf("nono: %s cannot be restricted inside %s on Linux",
					inner.path, outer.path)
			}
		}
	}
	return nil
}

type nonoProfile struct {
	Meta struct {
		Name string `json:"name"`
	} `json:"meta"`
	Groups struct {
		Exclude []string `json:"exclude,omitempty"`
	} `json:"groups"`
	Workdir struct {
		Access string `json:"access"`
	} `json:"workdir"`
	Filesystem struct {
		Allow []string `json:"allow,omitempty"`
		Read  []string `json:"read,omitempty"`
		Deny  []string `json:"deny,omitempty"`
	} `json:"filesystem"`
	Network struct {
		Block       bool     `json:"block"`
		AllowDomain []string `json:"allow_domain,omitempty"`
	} `json:"network"`
	Environment *struct {
		AllowVars []string `json:"allow_vars"`
	} `json:"environment,omitempty"`
}

func (sb *nono) profile(cmd Cmd) nonoProfile {
	var np nonoProfile
	np.Meta.Name = "gait"
	np.Groups.Exclude = sb.exclude
	np.Workdir.Access = "none"

	for _, r := range sb.hfs.rules {
		switch r.acc {
		case writeAccess:
			np.Filesystem.Allow = append(np.Filesystem.Allow, r.path)
		case readAccess:
			np.Filesystem.Read = append(np.Filesystem.Read, r.path)
		case noAccess:
			np.Filesystem.Deny = append(np.Filesystem.Deny, r.path)
		}
	}
	np.Filesystem.Allow = append(np.Filesystem.Allow, sb.devices...)

	switch sb.policy.Network.Mode {
	case NetNone:
		np.Network.Block = true
	case NetAllowList:
		np.Network.AllowDomain = sb.policy.Network.Hosts
	}

	if len(sb.policy.Env) > 0 {
		np.Environment = &struct {
			AllowVars []string `json:"allow_vars"`
		}{
			AllowVars: append(slices.Clone(sb.policy.Env),
				slices.Sorted(maps.Keys(cmd.Env))...),
		}
	}

	return np
}

func (sb *nono) Run(ctx context.Context, cmd Cmd) (Result, error) {
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

	f, err := os.CreateTemp(sb.tmpdir, "profile-*.json")
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(f.Name())

	err = json.NewEncoder(f).Encode(sb.profile(cmd))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Result{}, err
	}

	// The network proxy for an allow list needs nono's supervisor, which wrap does
	// not have.
	mode := "wrap"
	if sb.policy.Network.Mode == NetAllowList {
		mode = "run"
	}
	args := append([]string{"-s", mode, "--profile", f.Name(), "--workdir", dir, "--"},
		cmd.Args...)

	c := exec.CommandContext(ctx, sb.bin, args...)
	c.Dir = dir
	c.Env = os.Environ()
	for k, v := range cmd.Env {
		c.Env = append(c.Env, k+"="+v)
	}
	return runCombined(c)
}

func (sb *nono) FS() FS {
	return sb.hfs
}

func (sb *nono) Close(ctx context.Context) error {
	return os.RemoveAll(sb.tmpdir)
}
