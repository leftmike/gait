package system

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"testing"

	"github.com/leftmike/gait/config"
)

func TestDeniedReadWriteError(t *testing.T) {
	sb := checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{},
		Write: &config.RWActions{},
	}, nil)

	if _, err := sb.ReadFile("/etc/passwd"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile denied error = %v, want permission error", err)
	}
	if err := sb.WriteFile("/etc/passwd", nil, 0o644); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("WriteFile denied error = %v, want permission error", err)
	}
}

func TestAskFunc(t *testing.T) {
	cfg := &config.SandboxConfig{Read: &config.RWActions{Ask: []string{"/data/"}}}

	sb := checkSandbox(t, cfg, (&askRecorder{resp: false}).ask)
	if _, err := sb.ReadFile("/data/x.txt"); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile with declined ask = %v, want permission error", err)
	}

	// When granted, access proceeds to the OS read (which fails because the
	// file does not exist, not with a permission error).
	sb = checkSandbox(t, cfg, (&askRecorder{resp: true}).ask)
	if _, err := sb.ReadFile("/data/does-not-exist.txt"); errors.Is(err, fs.ErrPermission) {
		t.Errorf("ReadFile with granted ask returned permission error: %v", err)
	}
}

func TestNoAskFunc(t *testing.T) {
	// A nil ask func denies the operations that would ask, rather than
	// panicking on a nil call.
	sb := checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{Ask: []string{"/data/"}},
		Write: &config.RWActions{Ask: []string{"/data/"}},
	}, nil)
	wantPathError(t, sb.CheckRead("read", "/data/x.txt"), "read", "/data/x.txt")
	wantPathError(t, sb.CheckWrite("write", "/data/x.txt"), "write", "/data/x.txt")

	// A nil ask func with nothing to ask about is still unrestricted.
	sb = checkSandbox(t, nil, nil)
	if err := sb.CheckRead("read", "/data/x.txt"); err != nil {
		t.Errorf("CheckRead on an unrestricted sandbox failed: %s", err)
	}
}

func TestSandboxAskFuncIndependent(t *testing.T) {
	// Each sandbox carries its own ask func; two built from the same config
	// answer independently.
	cfg := &config.SandboxConfig{Read: &config.RWActions{Ask: []string{"/data/"}}}
	allowAsk, denyAsk := &askRecorder{resp: true}, &askRecorder{resp: false}
	allowSB := checkSandbox(t, cfg, allowAsk.ask)
	denySB := checkSandbox(t, cfg, denyAsk.ask)

	if err := allowSB.CheckRead("read", "/data/x.txt"); err != nil {
		t.Errorf("CheckRead on the allowing sandbox failed: %s", err)
	}
	wantPathError(t, denySB.CheckRead("read", "/data/x.txt"), "read", "/data/x.txt")

	if allowAsk.calls != 1 || denyAsk.calls != 1 {
		t.Errorf("ask calls = %d and %d, want 1 each", allowAsk.calls, denyAsk.calls)
	}
}

func (a action) String() string {
	switch a {
	case allow:
		return "allow"
	case ask:
		return "ask"
	default:
		return "deny"
	}
}

func TestAddPaths(t *testing.T) {
	var rwa rwActions

	err := rwa.addPaths([]string{"/etc/hosts", "/home/mike/", "/a/b/c.txt"}, allow)
	if err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	err = rwa.addPaths([]string{"/home/mike/.ssh/", "/etc/shadow"}, deny)
	if err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}

	wantFiles := map[string]action{
		"/etc/hosts":  allow,
		"/a/b/c.txt":  allow,
		"/etc/shadow": deny,
	}
	if !maps.Equal(rwa.files, wantFiles) {
		t.Errorf("addPaths files = %v, want %v", rwa.files, wantFiles)
	}

	wantDirs := []dirAction{{"/home/mike/", allow}, {"/home/mike/.ssh/", deny}}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("addPaths dirs = %v, want %v", rwa.dirs, wantDirs)
	}
}

func TestAddPathsEmpty(t *testing.T) {
	// Only directories: the files map is never allocated, and looking a path up
	// in the nil map must not panic.
	var rwa rwActions
	if err := rwa.addPaths([]string{"/data/"}, allow); err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	if rwa.files != nil {
		t.Errorf("addPaths files = %v, want nil", rwa.files)
	}
	if got := rwa.pathAction("/data/x.txt", deny); got != allow {
		t.Errorf("pathAction(/data/x.txt) = %s, want allow", got)
	}

	// No paths at all is not an error.
	if err := rwa.addPaths(nil, allow); err != nil {
		t.Errorf("addPaths(nil) failed: %s", err)
	}
}

func TestAddPathsDuplicateFile(t *testing.T) {
	var rwa rwActions
	if err := rwa.addPaths([]string{"/a/b"}, allow); err != nil {
		t.Fatalf("addPaths failed: %s", err)
	}
	if err := rwa.addPaths([]string{"/a/b"}, deny); err == nil {
		t.Error("addPaths with duplicate file path did not fail")
	}

	// Within a single call as well.
	var rwa2 rwActions
	if err := rwa2.addPaths([]string{"/a/b", "/a/b"}, allow); err == nil {
		t.Error("addPaths with repeated file path did not fail")
	}
}

func TestNewRWActionsNil(t *testing.T) {
	rwa, err := newRWActions(nil)
	if err != nil {
		t.Fatalf("newRWActions(nil) failed: %s", err)
	}
	if rwa != nil {
		t.Fatalf("newRWActions(nil) = %v, want nil", rwa)
	}
	// A nil *rwActions always yields the default.
	for _, dflt := range []action{deny, ask, allow} {
		if got := rwa.pathAction("/anything", dflt); got != dflt {
			t.Errorf("nil pathAction(/anything, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewRWActionsEmpty(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	if rwa == nil {
		t.Fatal("newRWActions(&config.RWActions{}) = nil, want non-nil")
	}
	if got := rwa.pathAction("/anything", ask); got != ask {
		t.Errorf("pathAction with no paths = %s, want ask", got)
	}
}

func TestPathAction(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/home/mike/", "/etc/hosts"},
		Ask:   []string{"/home/mike/secrets/", "/tmp/scratch"},
		Deny:  []string{"/home/mike/.ssh"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	cases := []struct {
		path string
		want action
	}{
		{"/etc/hosts", allow},               // exact file allow
		{"/tmp/scratch", ask},               // exact file ask
		{"/home/mike/.ssh", deny},           // exact file deny
		{"/home/mike/notes.txt", allow},     // beneath the allowed directory
		{"/home/mike/sub/notes.txt", allow}, // deeper beneath it
		{"/home/mike/.ssh/config", allow},   // a file rule does not cover children
		{"/etc/hostsX", deny},               // no prefix matching on a file rule
		{"/var/log/messages", deny},         // nothing matches -> default
		{"/home/mike", deny},                // the dir rule needs the trailing "/"
		{"/home/mike/secrets/keys", ask},    // more specific ask beats the allow
		{"/home/mike/secrets/a/b.txt", ask}, // deeper beneath the ask directory
	}

	for _, c := range cases {
		if got := rwa.pathAction(c.path, deny); got != c.want {
			t.Errorf("pathAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionClean(t *testing.T) {
	// Both the configured paths and the path under test are cleaned before
	// matching, so "." and ".." elements cannot slip past a rule.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/home/mike/"},
		Deny:  []string{"/home/mike/./secrets/keys/", "/home/mike/x/../.ssh"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	wantDirs := []dirAction{{"/home/mike/secrets/keys/", deny}, {"/home/mike/", allow}}
	if !slices.Equal(rwa.dirs, wantDirs) {
		t.Errorf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}

	cases := []struct {
		path string
		want action
	}{
		{"/home/mike/../mike/notes.txt", allow},
		{"/home/mike/./notes.txt", allow},
		{"/home/mike/secrets/keys/id_rsa", deny},
		{"/home/mike/../mike/secrets/keys/id_rsa", deny},
		{"/home/mike/secrets/../secrets/keys/id_rsa", deny},
		{"/home/mike/.ssh", deny},
		{"/home/mike/a/b/../../.ssh", deny},
	}

	for _, c := range cases {
		if got := rwa.pathAction(c.path, deny); got != c.want {
			t.Errorf("pathAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestPathActionRootDir(t *testing.T) {
	// Cleaning "/" must not turn it into "//".
	rwa, err := newRWActions(&config.RWActions{Allow: []string{"/"}})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	if wantDirs := []dirAction{{"/", allow}}; !slices.Equal(rwa.dirs, wantDirs) {
		t.Fatalf("dirs = %v, want %v", rwa.dirs, wantDirs)
	}
	for _, p := range []string{"/anything", "/a/b/c"} {
		if got := rwa.pathAction(p, deny); got != allow {
			t.Errorf("pathAction(%q) = %s, want allow", p, got)
		}
	}
}

func TestPathActionFileBeatsDir(t *testing.T) {
	// A file entry is consulted before any directory entry, whatever the
	// directory's action.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/data/"},
		Deny:  []string{"/data/secret.txt"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	if got := rwa.pathAction("/data/secret.txt", allow); got != deny {
		t.Errorf("pathAction(/data/secret.txt) = %s, want deny", got)
	}
	if got := rwa.pathAction("/data/other.txt", deny); got != allow {
		t.Errorf("pathAction(/data/other.txt) = %s, want allow", got)
	}
}

func TestPathActionDefault(t *testing.T) {
	rwa, err := newRWActions(&config.RWActions{Allow: []string{"/data/"}})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}
	// An unmatched path takes the default, whatever it is.
	for _, dflt := range []action{deny, ask, allow} {
		if got := rwa.pathAction("/elsewhere/x", dflt); got != dflt {
			t.Errorf("pathAction(/elsewhere/x, %s) = %s, want %s", dflt, got, dflt)
		}
	}
}

func TestNewRWActionsDuplicateFile(t *testing.T) {
	_, err := newRWActions(&config.RWActions{
		Allow: []string{"/a/b"},
		Deny:  []string{"/a/b"},
	})
	if err == nil {
		t.Error("newRWActions with a duplicate file path did not fail")
	}
}

func TestNewRWActionsDuplicateDir(t *testing.T) {
	_, err := newRWActions(&config.RWActions{
		Allow: []string{"/a/b/"},
		Deny:  []string{"/a/b/"},
	})
	if err == nil {
		t.Error("newRWActions with a duplicate directory did not fail")
	}

	// The duplicate is not adjacent in the order the paths are added, so
	// finding it depends on the directories being sorted first.
	_, err = newRWActions(&config.RWActions{
		Allow: []string{"/a/b/", "/c/d/"},
		Deny:  []string{"/a/b/"},
	})
	if err == nil {
		t.Error("newRWActions with a non-adjacent duplicate directory did not fail")
	}
}

func TestPathActionMostSpecificDir(t *testing.T) {
	// Nested directories: the longest matching directory determines the
	// action, regardless of which config list it came from.
	rwa, err := newRWActions(&config.RWActions{
		Allow: []string{"/home/mike/"},
		Ask:   []string{"/home/mike/secrets/"},
		Deny:  []string{"/home/mike/secrets/keys/"},
	})
	if err != nil {
		t.Fatalf("newRWActions failed: %s", err)
	}

	cases := []struct {
		path string
		want action
	}{
		{"/home/mike/notes.txt", allow},
		{"/home/mike/secrets/a.txt", ask},
		{"/home/mike/secrets/keys/id_rsa", deny},
	}

	for _, c := range cases {
		if got := rwa.pathAction(c.path, deny); got != c.want {
			t.Errorf("pathAction(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

// askRecorder is an AskFunc that always answers resp, recording the calls made
// and the arguments of the last one.
type askRecorder struct {
	resp     bool
	calls    int
	lastOp   string
	lastPath string
}

func (ar *askRecorder) ask(op, path string) bool {
	ar.calls += 1
	ar.lastOp, ar.lastPath = op, path
	return ar.resp
}

// wantPathError checks that err is a *fs.PathError carrying op, path and a
// permission error.
func wantPathError(t *testing.T, err error, op, path string) {
	t.Helper()

	var perr *fs.PathError
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v, want *fs.PathError", err)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v, want permission error", err)
	}
	if perr.Op != op {
		t.Errorf("error Op = %q, want %q", perr.Op, op)
	}
	if perr.Path != path {
		t.Errorf("error Path = %q, want %q", perr.Path, path)
	}
}

func checkSandbox(t *testing.T, cfg *config.SandboxConfig, ask AskFunc) *Sandbox {
	t.Helper()

	sb, err := NewSandbox(cfg, ask)
	if err != nil {
		t.Fatalf("NewSandbox failed: %s", err)
	}
	return sb
}

func TestCheckReadWrite(t *testing.T) {
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/", "/etc/hosts"}},
			Write: &config.RWActions{Allow: []string{"/data/out/"}},
		}, nil)

	// Reads are governed by the read actions.
	if err := sb.CheckRead("read", "/data/in.txt"); err != nil {
		t.Errorf("CheckRead(/data/in.txt) failed: %s", err)
	}
	if err := sb.CheckRead("read", "/etc/hosts"); err != nil {
		t.Errorf("CheckRead(/etc/hosts) failed: %s", err)
	}
	wantPathError(t, sb.CheckRead("read", "/etc/shadow"), "read", "/etc/shadow")

	// Writes are governed by the write actions, which are separate: /data/ is
	// readable but only /data/out/ is writable.
	if err := sb.CheckWrite("write", "/data/out/x.txt"); err != nil {
		t.Errorf("CheckWrite(/data/out/x.txt) failed: %s", err)
	}
	wantPathError(t, sb.CheckWrite("write", "/data/in.txt"), "write", "/data/in.txt")
	wantPathError(t, sb.CheckWrite("write", "/etc/hosts"), "write", "/etc/hosts")
}

func TestCheckReadWriteAsk(t *testing.T) {
	cfg := &config.SandboxConfig{
		Read:  &config.RWActions{Ask: []string{"/data/"}},
		Write: &config.RWActions{Ask: []string{"/data/"}},
	}

	// Granted: no error, and the op and path are passed through to the ask func.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t, cfg, ar.ask)
	if err := sb.CheckRead("read", "/data/x.txt"); err != nil {
		t.Errorf("CheckRead with granted ask failed: %s", err)
	}
	if ar.calls != 1 {
		t.Errorf("ask func called %d times, want 1", ar.calls)
	}
	if ar.lastOp != "read" || ar.lastPath != "/data/x.txt" {
		t.Errorf("ask func(%q, %q), want (%q, %q)", ar.lastOp, ar.lastPath, "read",
			"/data/x.txt")
	}

	if err := sb.CheckWrite("write", "/data/x.txt"); err != nil {
		t.Errorf("CheckWrite with granted ask failed: %s", err)
	}
	if ar.lastOp != "write" {
		t.Errorf("ask func op = %q, want %q", ar.lastOp, "write")
	}

	// Declined: a permission error.
	sb = checkSandbox(t, cfg, (&askRecorder{resp: false}).ask)
	wantPathError(t, sb.CheckRead("read", "/data/x.txt"), "read", "/data/x.txt")
	wantPathError(t, sb.CheckWrite("write", "/data/x.txt"), "write", "/data/x.txt")
}

func TestCheckReadWriteNoAsk(t *testing.T) {
	// The ask func must only be consulted for an ask action, never for allow or
	// for a denied path.
	ar := &askRecorder{resp: true}
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
			Write: &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
		}, ar.ask)

	if err := sb.CheckRead("read", "/data/x.txt"); err != nil {
		t.Errorf("CheckRead(/data/x.txt) failed: %s", err)
	}
	wantPathError(t, sb.CheckRead("read", "/data/no/x.txt"), "read", "/data/no/x.txt")
	wantPathError(t, sb.CheckWrite("write", "/elsewhere"), "write", "/elsewhere")

	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}
}

func TestCheckReadWriteUncleanPath(t *testing.T) {
	sb := checkSandbox(t,
		&config.SandboxConfig{
			Read:  &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
			Write: &config.RWActions{Allow: []string{"/data/"}, Deny: []string{"/data/no/"}},
		}, nil)

	// The path is cleaned before matching, so ".." cannot escape a deny.
	const unclean = "/data/yes/../no/x.txt"
	wantPathError(t, sb.CheckRead("read", unclean), "read", unclean)
	wantPathError(t, sb.CheckWrite("write", unclean), "write", unclean)

	// An unclean path that lands somewhere allowed is still allowed.
	if err := sb.CheckRead("read", "/data/./a/../x.txt"); err != nil {
		t.Errorf("CheckRead(/data/./a/../x.txt) failed: %s", err)
	}
}

func TestCheckReadWriteMissingConfig(t *testing.T) {
	// Within a configured sandbox, a config block that is absent leaves its
	// actions nil and denies everything.
	sb := checkSandbox(t, &config.SandboxConfig{
		Read: &config.RWActions{Allow: []string{"/data/"}},
	}, nil)
	if sb.writeActions != nil {
		t.Fatalf("writeActions = %v, want nil", sb.writeActions)
	}
	if err := sb.CheckRead("read", "/data/x.txt"); err != nil {
		t.Errorf("CheckRead(/data/x.txt) failed: %s", err)
	}
	wantPathError(t, sb.CheckWrite("write", "/data/x.txt"), "write", "/data/x.txt")

	// A config block with no paths in it denies everything too.
	sb = checkSandbox(t, &config.SandboxConfig{
		Read:  &config.RWActions{},
		Write: &config.RWActions{},
	}, nil)
	wantPathError(t, sb.CheckRead("read", "/anything"), "read", "/anything")
	wantPathError(t, sb.CheckWrite("write", "/anything"), "write", "/anything")
}

func TestCheckReadWriteNoSandbox(t *testing.T) {
	// No sandbox configured at all: unrestricted access, and the user is never
	// prompted.
	ar := &askRecorder{resp: false}
	sb := checkSandbox(t, nil, ar.ask)

	for _, p := range []string{"/anything", "/etc/shadow", "relative/path"} {
		if err := sb.CheckRead("read", p); err != nil {
			t.Errorf("CheckRead(%q) failed: %s", p, err)
		}
		if err := sb.CheckWrite("write", p); err != nil {
			t.Errorf("CheckWrite(%q) failed: %s", p, err)
		}
	}
	if ar.calls != 0 {
		t.Errorf("ask func called %d times, want 0", ar.calls)
	}
}
