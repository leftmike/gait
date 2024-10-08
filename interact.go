package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/peterh/liner"
	"github.com/teilomillet/gollm"
	"golang.org/x/term"
)

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func Interact(llm gollm.LLM, historyFilename string) error {
	line := liner.NewLiner()
	defer line.Close()

	writeHistory := true
	if f, err := os.Open(historyFilename); err == nil {
		_, err = line.ReadHistory(f)
		f.Close()
		if err != nil {
			writeHistory = false
		}
	}

	if writeHistory {
		defer func() {
			if f, err := os.Create(historyFilename); err != nil {
				fmt.Fprintf(os.Stderr, "%s: unable to write history to %s: %s", os.Args[0],
					historyFilename, err)
			} else {
				line.WriteHistory(f)
				f.Close()
			}
		}()
	}

	ctx := context.Background()
	for {
		s, err := line.Prompt("gait: ")
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		line.AppendHistory(s)

		s, err = llm.Generate(ctx, gollm.NewPrompt(s))
		if err != nil {
			return err
		}
		fmt.Println(s, "\n")
	}

	return nil
}
