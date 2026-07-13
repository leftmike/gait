package tool

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/leftmike/sandbox"
)

func fsPolicy(sb *sandbox.Sandbox) *sandbox.FSPolicy {
	if sb == nil {
		return nil
	}

	return sb.FSP
}

// pathsAllow reports whether path is one of paths or is contained in one of
// them, mirroring how sandbox.FSPolicy matches paths.
func pathsAllow(paths []string, path string) bool {
	path += "/"
	for _, allow := range paths {
		if !strings.HasSuffix(allow, "/") {
			allow += "/"
		}
		if strings.HasPrefix(path, allow) {
			return true
		}
	}
	return false
}

// checkRead returns an error if the sandbox's filesystem policy denies reading
// path. Paths granted write or execute access may be read as well.
func checkRead(sb *sandbox.Sandbox, path string) error {
	fsp := fsPolicy(sb)
	if fsp == nil {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if pathsAllow(fsp.Read, abs) || pathsAllow(fsp.Write, abs) ||
		pathsAllow(fsp.Execute, abs) {

		return nil
	}
	return fmt.Errorf("sandbox: read access denied: %s", path)
}

// checkWrite returns an error if the sandbox's filesystem policy denies
// writing path; writing includes creating, truncating, and removing it.
func checkWrite(sb *sandbox.Sandbox, path string) error {
	fsp := fsPolicy(sb)
	if fsp == nil {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if pathsAllow(fsp.Write, abs) {
		return nil
	}
	return fmt.Errorf("sandbox: write access denied: %s", path)
}

// combinedOutput runs a command in dir and returns its combined stdout and
// stderr, executing it inside the sandbox when one is provided.
func combinedOutput(ctx context.Context, sb *sandbox.Sandbox, dir, name string,
	arg ...string) ([]byte, error) {

	if sb == nil {
		cmd := exec.CommandContext(ctx, name, arg...)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}

	cmd := sandbox.CommandContext(ctx, name, arg...)
	cmd.Sandbox = sb
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
