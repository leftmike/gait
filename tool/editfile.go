package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
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

type editFileArgs struct {
	Path       string `json:"path" gait:"the path of the file to edit"`
	OldString  string `json:"old_string" gait:"the text to replace"`
	NewString  string `json:"new_string" gait:"the text to replace it with (must differ from old_string)"`
	ReplaceAll bool   `json:"replace_all,omitempty" gait:"replace all occurrences of old_string (default false)"`
}

func editFile(ctx context.Context, buf []byte) (string, error) {
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

func EditFile() Tool {
	return Tool{
		Name: "edit_file",
		Description: "Performs exact string replacement in a text file. Unless replace_all is " +
			"set, old_string must match exactly once, so include enough surrounding " +
			"context to make it unique.",
		Func:   editFile,
		Schema: MustToolSchema[editFileArgs](),
	}
}
