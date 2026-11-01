//go:build microsandbox

package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func TestMicrosandboxMounts(t *testing.T) {
	dir := tempDir(t)
	ro := tempDir(t)
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Policy{
		Workdir:  dir,
		ReadOnly: []string{ro, "/no/such/path"},
		Deny:     []string{secret, filepath.Join(dir, "missing"), filepath.Join(dir, "sub")},
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	hfs, err := NewHostFS(p)
	if err != nil {
		t.Fatal(err)
	}
	sb := &microsandbox{hfs: hfs, tmpdir: tempDir(t)}

	mounts, err := sb.mounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 5 {
		t.Errorf("mounts = %v, want 5", mounts)
	}
	if m := mounts[dir]; m.Bind != dir || m.Readonly {
		t.Errorf("workdir mount = %+v", m)
	}
	if m := mounts[ro]; m.Bind != ro || !m.Readonly {
		t.Errorf("read-only mount = %+v", m)
	}
	if m := mounts[secret]; m.Bind == "" || m.Bind == secret || !m.Readonly {
		t.Errorf("denied file mount = %+v", m)
	}
	for _, path := range []string{filepath.Join(dir, "missing"), filepath.Join(dir, "sub")} {
		if m := mounts[path]; !m.Tmpfs || m.Readonly != true {
			t.Errorf("denied directory mount %s = %+v", path, m)
		}
	}

	hfs, err = NewHostFS(Policy{Workdir: dir, ReadOnly: []string{"/"}})
	if err != nil {
		t.Fatal(err)
	}
	sb.hfs = hfs
	if _, err := sb.mounts(); err == nil {
		t.Error("mounting / did not fail")
	}
}

func TestMicrosandboxNetwork(t *testing.T) {
	if nc := networkConfig(Network{}); nc.DefaultEgress != msb.PolicyActionDeny ||
		len(nc.Rules) != 0 {

		t.Errorf("none = %+v", nc)
	}
	if nc := networkConfig(Network{Mode: NetAll}); nc.DefaultEgress != msb.PolicyActionAllow {
		t.Errorf("all = %+v", nc)
	}
	nc := networkConfig(Network{Mode: NetAllowList, Hosts: []string{"example.com"}})
	if nc.DefaultEgress != msb.PolicyActionDeny || len(nc.Rules) != 2 ||
		nc.Rules[1].Destination != "example.com" {

		t.Errorf("allow = %+v", nc)
	}
}
