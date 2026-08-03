package system

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/leftmike/gait/config"
)

// AskFunc is called to ask the user whether an operation may proceed. For
// reads and writes, what is passed is the path; for executes, the command
// line.
type AskFunc func(op, what string) bool

type action int

const (
	deny action = iota
	ask
	allow
)

type dirAction struct {
	dir string
	act action
}

type rwActions struct {
	files map[string]action
	dirs  []dirAction
}

// resolveSymlinks resolves the symbolic links in path, so that the path which
// is checked against the configured paths is the path which the operation will
// actually reach. Without this, a link within an allowed directory reaches a
// file outside of it, and a link to a denied command runs it.
//
// Only the links which exist can be resolved; trailing components which do not
// exist yet are kept as they are, so that a path can be checked before it is
// created. Resolving here still leaves the window between the check and the
// operation, in which a component could be replaced by a link; closing that
// needs the operations themselves to refuse to follow links.
func resolveSymlinks(path string) string {
	var rest string

	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, rest)
		}

		dir := filepath.Dir(path)
		if dir == path {
			// The root was reached without resolving anything.
			return filepath.Join(path, rest)
		}

		rest = filepath.Join(filepath.Base(path), rest)
		path = dir
	}
}

func expandPath(path, cwd, home string, look bool) (string, bool, error) {
	hasSuffix := strings.HasSuffix(path, "/")

	if filepath.IsAbs(path) {
		path = filepath.Clean(path)
	} else if path == "." {
		path = cwd
	} else if strings.HasPrefix(path, "./") {
		path = filepath.Join(cwd, strings.TrimPrefix(path, "./"))
	} else if path == "~" {
		path = home
	} else if strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	} else if strings.ContainsRune(path, '/') || !look {
		return "", false, fmt.Errorf("sandbox: relative paths must start with ./: %s", path)
	} else {
		var err error
		path, err = exec.LookPath(path)
		if err != nil {
			return "", false, fmt.Errorf("sandbox: %s", err)
		}
		path = filepath.Clean(path)
	}

	path = resolveSymlinks(path)

	fi, err := os.Stat(path)
	if err != nil {
		return path, hasSuffix, nil
	}

	isDir := fi.Mode().IsDir()
	if hasSuffix && !isDir {
		return "", false, fmt.Errorf("sandbox: path has trailing / and is a file: %s", path)
	}

	return path, hasSuffix || isDir, nil
}

func (rwa *rwActions) addPaths(paths []string, act action, cwd, home string) error {
	for _, path := range paths {
		path, isDir, err := expandPath(path, cwd, home, false)
		if err != nil {
			return err
		}

		if isDir {
			if path != "/" {
				path += "/"
			}
			rwa.dirs = append(rwa.dirs, dirAction{path, act})
		} else {
			if _, ok := rwa.files[path]; ok {
				return fmt.Errorf("sandbox: path specified more than once: %s", path)
			}

			if rwa.files == nil {
				rwa.files = map[string]action{}
			}
			rwa.files[path] = act
		}
	}

	return nil
}

func newRWActions(cfg *config.RWActions, cwd, home string) (*rwActions, error) {
	if cfg == nil {
		return nil, nil
	}

	var rwa rwActions
	err := rwa.addPaths(cfg.Allow, allow, cwd, home)
	if err != nil {
		return nil, err
	}
	err = rwa.addPaths(cfg.Ask, ask, cwd, home)
	if err != nil {
		return nil, err
	}
	err = rwa.addPaths(cfg.Deny, deny, cwd, home)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(rwa.dirs, func(da1, da2 dirAction) int {
		return -strings.Compare(da1.dir, da2.dir)
	})

	for i := 1; i < len(rwa.dirs); i += 1 {
		if rwa.dirs[i-1].dir == rwa.dirs[i].dir {
			return nil, fmt.Errorf("sandbox: path specified more than once: %s", rwa.dirs[i].dir)
		}
	}

	return &rwa, nil
}

// resolveAction returns the action configured for path, which must already have
// been expanded: expandPath cleans both the configured paths and the path being
// checked, so the two are compared in the same terms.
func (rwa *rwActions) resolveAction(path string, dflt action) action {
	if rwa == nil {
		return dflt
	}

	act, ok := rwa.files[path]
	if ok {
		return act
	}

	pathSlash := path + "/"
	for _, da := range rwa.dirs {
		// A directory covers everything within it as well as the directory
		// itself, so that the directory can be created and removed.
		if strings.HasPrefix(path, da.dir) || pathSlash == da.dir {
			return da.act
		}
	}

	return dflt
}

type cmdAction struct {
	pats []*regexp.Regexp
	act  action
}

type execActions struct {
	cmds map[string][]cmdAction
	dirs []dirAction
}

func (ea *execActions) addCmds(cmds [][]string, act action, cwd, home string) error {
	for _, cmd := range cmds {
		if len(cmd) == 0 {
			return errors.New("sandbox: execute action must specify a command")
		}

		// A bare command name is looked up on PATH, so that a command configured
		// by name and the same command run by name or by path are compared in
		// the same terms.
		path, isDir, err := expandPath(cmd[0], cwd, home, true)
		if err != nil {
			return err
		}

		if isDir {
			if len(cmd) > 1 {
				return fmt.Errorf("sandbox: arguments not allowed with a directory: %s", cmd[0])
			}

			if path != "/" {
				path += "/"
			}
			ea.dirs = append(ea.dirs, dirAction{path, act})
		} else {
			var pats []*regexp.Regexp
			for _, arg := range cmd[1:] {
				// A pattern must match the whole argument. An unanchored
				// pattern is satisfied by any argument merely containing a
				// match, which would let an allow rule pass the arguments it
				// was written to exclude.
				pat, err := regexp.Compile(`\A(?:` + arg + `)\z`)
				if err != nil {
					return fmt.Errorf("sandbox: %s: %s", cmd[0], err)
				}
				pats = append(pats, pat)
			}

			found := slices.ContainsFunc(ea.cmds[path], func(ca cmdAction) bool {
				if len(ca.pats) != len(pats) {
					return false
				}

				for i, pat := range ca.pats {
					if pat.String() != pats[i].String() {
						return false
					}
				}

				return true
			})
			if found {
				return fmt.Errorf("sandbox: duplicate command: %s", strings.Join(cmd, " "))
			}

			if ea.cmds == nil {
				ea.cmds = map[string][]cmdAction{}
			}
			ea.cmds[path] = append(ea.cmds[path], cmdAction{pats, act})
		}
	}

	return nil
}

func newExecActions(cfg *config.ExecuteActions, cwd, home string) (*execActions, error) {
	if cfg == nil {
		return nil, nil
	}

	var ea execActions
	err := ea.addCmds(cfg.Allow, allow, cwd, home)
	if err != nil {
		return nil, err
	}
	err = ea.addCmds(cfg.Ask, ask, cwd, home)
	if err != nil {
		return nil, err
	}
	err = ea.addCmds(cfg.Deny, deny, cwd, home)
	if err != nil {
		return nil, err
	}

	// The most specific rule for a command is the one constraining the most
	// arguments; of equally specific rules, the most restrictive applies.
	for _, cas := range ea.cmds {
		slices.SortStableFunc(cas, func(ca1, ca2 cmdAction) int {
			if n := len(ca2.pats) - len(ca1.pats); n != 0 {
				return n
			}
			return int(ca1.act) - int(ca2.act)
		})
	}

	slices.SortFunc(ea.dirs, func(da1, da2 dirAction) int {
		return -strings.Compare(da1.dir, da2.dir)
	})

	for i := 1; i < len(ea.dirs); i += 1 {
		if ea.dirs[i-1].dir == ea.dirs[i].dir {
			return nil, fmt.Errorf("sandbox: directory specified more than once: %s",
				ea.dirs[i].dir)
		}
	}

	return &ea, nil
}

// resolveAction returns the action configured for path, which must already have
// been expanded: expandPath resolves both the configured commands and the
// command being run, so the two are compared in the same terms.
func (ea *execActions) resolveAction(path string, args []string, dflt action) action {
	if ea == nil {
		return dflt
	}

	// A rule for the command itself is more specific than a rule for the
	// directory it is in; a rule whose patterns don't match is not a rule for
	// this command at all.
	for _, ca := range ea.cmds[path] {
		if len(ca.pats) <= len(args) {
			i := 0
			for {
				if i == len(ca.pats) {
					return ca.act
				} else if !ca.pats[i].MatchString(args[i]) {
					break
				}

				i += 1
			}
		}
	}

	for _, da := range ea.dirs {
		if strings.HasPrefix(path, da.dir) {
			return da.act
		}
	}

	return dflt
}

type Sandbox struct {
	cwd            string
	home           string
	readActions    *rwActions
	writeActions   *rwActions
	executeActions *execActions
	dflt           action
	ask            AskFunc
}

func NewSandbox(sbCfg *config.SandboxConfig, ask AskFunc) (*Sandbox, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("sandbox: %s", err)
	}
	cwd = filepath.Clean(cwd)

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("sandbox: %s", err)
	}
	home = filepath.Clean(home)

	if sbCfg == nil {
		return &Sandbox{
			cwd:  cwd,
			home: home,
			dflt: allow,
			ask:  ask,
		}, nil
	}

	readActions, err := newRWActions(sbCfg.Read, cwd, home)
	if err != nil {
		return nil, err
	}
	writeActions, err := newRWActions(sbCfg.Write, cwd, home)
	if err != nil {
		return nil, err
	}
	executeActions, err := newExecActions(sbCfg.Execute, cwd, home)
	if err != nil {
		return nil, err
	}

	return &Sandbox{
		cwd:            cwd,
		home:           home,
		readActions:    readActions,
		writeActions:   writeActions,
		executeActions: executeActions,
		dflt:           deny,
		ask:            ask,
	}, nil
}

// checkAction checks that op is allowed on path, which must already have been
// expanded, asking the user if that is what the path is configured for.
func (sb *Sandbox) checkAction(rwa *rwActions, op, path string) error {
	switch rwa.resolveAction(path, sb.dflt) {
	case allow:
		return nil
	case ask:
		if sb.ask != nil && sb.ask(op, path) {
			return nil
		}
	}

	return &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission}
}

// expandCheckRW expands path the same way as the configured paths were
// expanded, so that the two are compared in the same terms, and checks that op
// is allowed on it. The expanded path is what the operation must use.
func (sb *Sandbox) expandCheckRW(rwa *rwActions, op, path string) (string, error) {
	path, _, err := expandPath(path, sb.cwd, sb.home, false)
	if err != nil {
		return "", err
	}

	if err := sb.checkAction(rwa, op, path); err != nil {
		return "", err
	}

	return path, nil
}

func (sb *Sandbox) ReadFile(path string) ([]byte, error) {
	path, err := sb.expandCheckRW(sb.readActions, "read", path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (sb *Sandbox) WriteFile(path string, data []byte, perm fs.FileMode) error {
	path, err := sb.expandCheckRW(sb.writeActions, "write", path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func (sb *Sandbox) MkdirAll(path string, perm fs.FileMode) error {
	path, err := sb.expandCheckRW(sb.writeActions, "mkdir", path)
	if err != nil {
		return err
	}

	// The missing parents are created as well, so each of them must be allowed
	// too; checking only the path itself would create a denied directory on the
	// way to an allowed one. A parent which does not exist cannot be a symlink,
	// so what is checked is still what is created.
	var missing []string
	for dir := filepath.Dir(path); ; {
		if _, err := os.Stat(dir); err == nil {
			break
		}

		missing = append(missing, dir)

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// The parents are checked in the order in which they will be created, so
	// that asking about them follows the same order.
	for i := len(missing) - 1; i >= 0; i -= 1 {
		err := sb.checkAction(sb.writeActions, "mkdir", missing[i])
		if err != nil {
			return err
		}
	}

	return os.MkdirAll(path, perm)
}

func (sb *Sandbox) Stat(path string) (fs.FileInfo, error) {
	path, err := sb.expandCheckRW(sb.readActions, "stat", path)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (sb *Sandbox) Remove(path string) error {
	path, err := sb.expandCheckRW(sb.writeActions, "remove", path)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (sb *Sandbox) WalkDir(root string, fn fs.WalkDirFunc) error {
	root, err := sb.expandCheckRW(sb.readActions, "walkdir", root)
	if err != nil {
		return err
	}

	return filepath.WalkDir(root,
		func(path string, d fs.DirEntry, err error) error {
			// Denied paths are skipped rather than failing the whole walk, so
			// that a denied file or subdirectory is simply not visible.
			// Anything below root which needs asking is walked without asking;
			// reading it is still checked.
			if sb.readActions.resolveAction(path, sb.dflt) == deny {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			return fn(path, d, err)
		})
}

// quoteArg quotes an argument which would otherwise be unclear, leaving the
// ordinary ones as they are so that the command line stays readable.
func quoteArg(arg string) string {
	if arg == "" || !utf8.ValidString(arg) {
		return strconv.Quote(arg)
	}

	for _, r := range arg {
		if r == ' ' || r == '"' || r == '\\' || !strconv.IsPrint(r) {
			return strconv.Quote(arg)
		}
	}

	return arg
}

// quoteCmdline renders a command line for the user to be asked about. The
// arguments are quoted so that what is shown is unambiguous: an argument
// containing spaces is not mistaken for several arguments, an empty one is
// visible, and one containing newlines or other control characters cannot
// forge further lines in the prompt.
func quoteCmdline(path string, args []string) string {
	var b strings.Builder

	b.WriteString(quoteArg(path))
	for _, arg := range args {
		b.WriteByte(' ')
		b.WriteString(quoteArg(arg))
	}

	return b.String()
}

// expandCheckExec expands name the same way as the configured commands were
// expanded, so that the two are compared in the same terms, and checks that it
// may be executed. The expanded path is what must be executed.
func (sb *Sandbox) expandCheckExec(name string, args []string) (string, error) {
	path, _, err := expandPath(name, sb.cwd, sb.home, true)
	if err != nil {
		return "", err
	}

	cmdline := quoteCmdline(path, args)

	switch sb.executeActions.resolveAction(path, args, sb.dflt) {
	case allow:
		return path, nil
	case ask:
		if sb.ask != nil && sb.ask("execute", cmdline) {
			return path, nil
		}
	}

	return "", &fs.PathError{Op: "execute", Path: cmdline, Err: fs.ErrPermission}
}

// CombinedOutput runs a command in dir and returns its combined stdout and
// stderr.
func (sb *Sandbox) CombinedOutput(ctx context.Context, dir, name string, arg ...string) ([]byte,
	error) {

	path, err := sb.expandCheckExec(name, arg)
	if err != nil {
		return nil, err
	}

	// The expanded path is run, rather than the name as given, so that what is
	// executed is what was checked.
	cmd := exec.CommandContext(ctx, path, arg...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
