package system

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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

// pathAction returns the action configured for path, which must already have
// been expanded: expandPath cleans both the configured paths and the path being
// checked, so the two are compared in the same terms.
func (rwa *rwActions) pathAction(path string, dflt action) action {
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

// lookPath resolves a command to the absolute path which will actually be
// executed, so that the configured commands and the commands being run are
// compared in the same terms. A command which can not be resolved is used as
// given.
func lookPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		path = name
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	return filepath.Clean(path)
}

// argPatterns is one rule for a command: the arguments must match patterns for
// act to apply. The patterns match the leading arguments, one pattern per
// argument, leaving any remaining arguments unconstrained; no patterns at all
// matches the command however it is called.
type argPatterns struct {
	patterns []*regexp.Regexp
	act      action
}

func (ap argPatterns) match(args []string) bool {
	if len(ap.patterns) > len(args) {
		return false
	}

	for i, re := range ap.patterns {
		if !re.MatchString(args[i]) {
			return false
		}
	}

	return true
}

func (ap argPatterns) equal(patterns []*regexp.Regexp) bool {
	if len(ap.patterns) != len(patterns) {
		return false
	}

	for i, re := range ap.patterns {
		if re.String() != patterns[i].String() {
			return false
		}
	}

	return true
}

type execActions struct {
	cmds map[string][]argPatterns
	dirs []dirAction
}

func (ea *execActions) addCmds(cmds [][]string, act action) error {
	for _, cmd := range cmds {
		if len(cmd) == 0 {
			return fmt.Errorf("sandbox: execute action must specify a command")
		}

		if strings.HasSuffix(cmd[0], "/") {
			if len(cmd) > 1 {
				return fmt.Errorf("sandbox: arguments not allowed with a directory: %s", cmd[0])
			}

			dir := lookPath(cmd[0])
			if dir != "/" {
				dir += "/"
			}
			ea.dirs = append(ea.dirs, dirAction{dir, act})
			continue
		}

		var patterns []*regexp.Regexp
		for _, arg := range cmd[1:] {
			re, err := regexp.Compile(arg)
			if err != nil {
				return fmt.Errorf("sandbox: %s: %s", cmd[0], err)
			}
			patterns = append(patterns, re)
		}

		path := lookPath(cmd[0])
		if slices.ContainsFunc(ea.cmds[path],
			func(ap argPatterns) bool {
				return ap.equal(patterns)
			}) {

			return fmt.Errorf("sandbox: command specified more than once: %s",
				strings.Join(cmd, " "))
		}

		if ea.cmds == nil {
			ea.cmds = map[string][]argPatterns{}
		}
		ea.cmds[path] = append(ea.cmds[path], argPatterns{patterns, act})
	}

	return nil
}

func newExecActions(cfg *config.ExecuteActions) (*execActions, error) {
	if cfg == nil {
		return nil, nil
	}

	var ea execActions
	err := ea.addCmds(cfg.Allow, allow)
	if err != nil {
		return nil, err
	}
	err = ea.addCmds(cfg.Ask, ask)
	if err != nil {
		return nil, err
	}
	err = ea.addCmds(cfg.Deny, deny)
	if err != nil {
		return nil, err
	}

	// The most specific rule for a command is the one constraining the most
	// arguments; of equally specific rules, the most restrictive applies.
	for _, aps := range ea.cmds {
		slices.SortStableFunc(aps, func(ap1, ap2 argPatterns) int {
			if n := len(ap2.patterns) - len(ap1.patterns); n != 0 {
				return n
			}
			return int(ap1.act) - int(ap2.act)
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

func (ea *execActions) cmdAction(name string, args []string, dflt action) action {
	if ea == nil {
		return dflt
	}

	path := lookPath(name)

	// A rule for the command itself is more specific than a rule for the
	// directory it is in; a rule whose patterns don't match is not a rule for
	// this command at all.
	for _, ap := range ea.cmds[path] {
		if ap.match(args) {
			return ap.act
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
	executeActions, err := newExecActions(sbCfg.Execute) // XXX
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

// expandCheckRW expands path the same way as the configured paths were
// expanded, so that the two are compared in the same terms, and checks that op
// is allowed on it. The expanded path is what the operation must use.
func (sb *Sandbox) expandCheckRW(rwa *rwActions, op, path string) (string, error) {
	path, _, err := expandPath(path, sb.cwd, sb.home, false)
	if err != nil {
		return "", err
	}

	switch rwa.pathAction(path, sb.dflt) {
	case allow:
		return path, nil
	case ask:
		if sb.ask != nil && sb.ask(op, path) {
			return path, nil
		}
	}

	return "", &fs.PathError{Op: op, Path: path, Err: fs.ErrPermission}
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
			if sb.readActions.pathAction(path, sb.dflt) == deny {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			return fn(path, d, err)
		})
}

func (sb *Sandbox) checkExec(name string, args []string) error {
	cmdline := strings.Join(append([]string{name}, args...), " ")

	switch sb.executeActions.cmdAction(name, args, sb.dflt) {
	case allow:
		return nil
	case ask:
		if sb.ask != nil && sb.ask("execute", cmdline) {
			return nil
		}
	}

	return &fs.PathError{Op: "execute", Path: cmdline, Err: fs.ErrPermission}
}

// CombinedOutput runs a command in dir and returns its combined stdout and
// stderr.
func (sb *Sandbox) CombinedOutput(ctx context.Context, dir, name string, arg ...string) ([]byte,
	error) {

	if err := sb.checkExec(name, arg); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
