package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/leftmike/gait/sandbox"
)

type writeFileArgs struct {
	Path    string `json:"path" gait:"the path of the file to write"`
	Content string `json:"content" gait:"the content to write to the file"`
}

func writeFile(ctx context.Context, sb sandbox.Sandbox, buf []byte) (string, error) {
	var args writeFileArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	if dir := filepath.Dir(args.Path); dir != "" {
		err = sb.FS().MkdirAll(ctx, dir, 0755)
		if err != nil {
			return "", err
		}
	}

	err = sb.FS().WriteFile(ctx, args.Path, []byte(args.Content), 0644)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}

var WriteFile = Tool{
	Name: "write_file",
	Description: "Writes a text file to the local filesystem, overwriting it if it already " +
		"exists. Creates parent directories as needed.",
	Func:   writeFile,
	Schema: MustToolSchema[writeFileArgs](),
}
