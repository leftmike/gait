package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/leftmike/gait/model"
)

// isBinary reports whether data looks like a binary (non-text) file. It uses
// the same heuristic as git: a NUL byte in the first chunk means binary.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

type writeFileArgs struct {
	Path    string `json:"path" gait:"the path of the file to write"`
	Content string `json:"content" gait:"the content to write to the file"`
}

func (ag *Agent) writeFile(ctx context.Context, buf []byte) (string, error) {
	var args writeFileArgs
	if err := json.Unmarshal(buf, &args); err != nil {
		return "", err
	}

	if dir := filepath.Dir(args.Path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}

	if err := os.WriteFile(args.Path, []byte(args.Content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}

func (ag *Agent) AddWriteFileTool() {
	if _, ok := ag.Tools["write_file"]; !ok {
		ag.AddTool("write_file",
			"Writes a text file to the local filesystem, overwriting it if it already "+
				"exists. Creates parent directories as needed.",
			ag.writeFile, model.MustToolSchema[writeFileArgs]())
	}
}

type editFileArgs struct {
	Path       string `json:"path" gait:"the path of the file to edit"`
	OldString  string `json:"old_string" gait:"the text to replace"`
	NewString  string `json:"new_string" gait:"the text to replace it with (must differ from old_string)"`
	ReplaceAll bool   `json:"replace_all,omitempty" gait:"replace all occurrences of old_string (default false)"`
}

func (ag *Agent) editFile(ctx context.Context, buf []byte) (string, error) {
	var args editFileArgs
	if err := json.Unmarshal(buf, &args); err != nil {
		return "", err
	}

	if args.OldString == "" {
		return "", fmt.Errorf("old_string must not be empty")
	}
	if args.OldString == args.NewString {
		return "", fmt.Errorf("old_string and new_string are identical")
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", err
	}
	if isBinary(data) {
		return "", fmt.Errorf("cannot edit binary file: %s", args.Path)
	}

	content := string(data)
	count := strings.Count(content, args.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", args.Path)
	}

	var updated string
	if args.ReplaceAll {
		updated = strings.ReplaceAll(content, args.OldString, args.NewString)
	} else {
		if count > 1 {
			return "", fmt.Errorf(
				"old_string is not unique in %s (%d occurrences); "+
					"provide more surrounding context or set replace_all",
				args.Path, count)
		}
		updated = strings.Replace(content, args.OldString, args.NewString, 1)
	}

	mode := fs.FileMode(0644)
	if info, err := os.Stat(args.Path); err == nil {
		mode = info.Mode()
	}
	if err := os.WriteFile(args.Path, []byte(updated), mode); err != nil {
		return "", err
	}

	if count == 1 {
		return fmt.Sprintf("made 1 replacement in %s", args.Path), nil
	}
	return fmt.Sprintf("made %d replacements in %s", count, args.Path), nil
}

func (ag *Agent) AddEditFileTool() {
	if _, ok := ag.Tools["edit_file"]; !ok {
		ag.AddTool("edit_file",
			"Performs exact string replacement in a text file. Unless replace_all is "+
				"set, old_string must match exactly once, so include enough surrounding "+
				"context to make it unique.",
			ag.editFile, model.MustToolSchema[editFileArgs]())
	}
}
