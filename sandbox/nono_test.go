package sandbox

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestLandlockConflicts(t *testing.T) {
	cases := []struct {
		rules []rule
		fail  bool
	}{
		{rules: []rule{{"/w", writeAccess}, {"/r", readAccess}, {"/d", noAccess}}},
		{rules: []rule{{"/r", readAccess}, {"/r/w", writeAccess}}},
		{rules: []rule{{"/w", writeAccess}, {"/wx", noAccess}}},
		{rules: []rule{{"/w", writeAccess}, {"/w/d", noAccess}}, fail: true},
		{rules: []rule{{"/w", writeAccess}, {"/w/r", readAccess}}, fail: true},
		{rules: []rule{{"/r", readAccess}, {"/r/a/d", noAccess}}, fail: true},
		{rules: []rule{{"/", readAccess}, {"/d", noAccess}}, fail: true},
	}

	for _, c := range cases {
		err := landlockConflicts(c.rules)
		if c.fail && err == nil {
			t.Errorf("landlockConflicts(%v) did not fail", c.rules)
		} else if !c.fail && err != nil {
			t.Errorf("landlockConflicts(%v) failed: %s", c.rules, err)
		}
	}
}

func TestNonoProfile(t *testing.T) {
	dir := tempDir(t)
	p, err := Policy{
		Workdir:  dir,
		ReadOnly: []string{"/data"},
		Deny:     []string{"/secret"},
		Network:  Network{Mode: NetAllowList, Hosts: []string{"example.com"}},
		Env:      []string{"PATH"},
	}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	hfs, err := NewHostFS(p)
	if err != nil {
		t.Fatal(err)
	}
	sb := &nono{policy: p, hfs: hfs, exclude: []string{"g"}, devices: []string{"/dev/null"}}

	np := sb.profile(Cmd{Env: map[string]string{"B": "1", "A": "2"}})
	if np.Workdir.Access != "none" || !slices.Equal(np.Groups.Exclude, []string{"g"}) {
		t.Errorf("profile = %+v", np)
	}
	if !slices.Equal(np.Filesystem.Allow, []string{dir, "/dev/null"}) ||
		!slices.Equal(np.Filesystem.Read, []string{"/data"}) ||
		!slices.Equal(np.Filesystem.Deny, []string{"/secret"}) {

		t.Errorf("filesystem = %+v", np.Filesystem)
	}
	if np.Network.Block || !slices.Equal(np.Network.AllowDomain, []string{"example.com"}) {
		t.Errorf("network = %+v", np.Network)
	}
	if np.Environment == nil ||
		!slices.Equal(np.Environment.AllowVars, []string{"PATH", "A", "B"}) {

		t.Errorf("environment = %+v", np.Environment)
	}

	sb.policy.Network = Network{}
	sb.policy.Env = nil
	np = sb.profile(Cmd{})
	if !np.Network.Block || np.Environment != nil {
		t.Errorf("profile = %+v", np)
	}
}

func TestNonoRunOutsidePolicy(t *testing.T) {
	if _, err := exec.LookPath("nono"); err != nil {
		t.Skip("nono not installed")
	}

	ctx := context.Background()
	sb, err := Open(ctx, "nono", Policy{Workdir: tempDir(t)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close(ctx)

	_, err = sb.Run(ctx, Cmd{Args: []string{"pwd"}, Dir: filepath.Dir(tempDir(t))})
	wantPermission(t, err, "Run outside the policy")
}
