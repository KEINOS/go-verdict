package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/appver"
	"github.com/KEINOS/go-verdict/cmd/verdict/internal/hotspot"
	"github.com/KEINOS/go-verdict/cmd/verdict/internal/skill"
)

const (
	cmdHotspot = "hotspot"
	cmdSkill   = "skill"
	cmdVersion = "version"
)

type subcmd interface {
	Run() (string, error)
}

func runTopLevelCommand(args []string, output io.Writer) (bool, error) {
	const (
		commandHandled   = true  // Args consumed as a top-level command.
		commandUnhandled = false // Continue with normal CLI parsing.
	)

	command, hasSubCommand := topLevelCommand(args)
	if !hasSubCommand {
		return commandUnhandled, nil
	}

	if command != cmdHotspot && command != cmdSkill && command != cmdVersion {
		return commandHandled, fmt.Errorf("%w: %s", errUnknownCommand, command)
	}

	if command == cmdHotspot {
		err := hotspot.New().Run(args[1:], output)
		if err != nil {
			return commandHandled, fmt.Errorf("running hotspot command: %w", err)
		}

		return commandHandled, nil
	}

	if len(args) != 1 {
		return commandHandled, errUnexpectedCommandArgs
	}

	var subCommand subcmd

	if command == cmdSkill {
		subCommand = skill.New()
	} else {
		subCommand = appver.New()
	}

	return commandHandled, runSubcmd(subCommand, output, command == cmdVersion)
}

func runSubcmd(command subcmd, output io.Writer, addTrailingNewline bool) error {
	if command == nil {
		return errUnknownCommand
	}

	text, err := command.Run()
	if err != nil {
		return fmt.Errorf("running subcommand: %w", err)
	}

	if addTrailingNewline {
		text += "\n"
	}

	return writeString(output, text)
}

func runVersionCommand(output io.Writer) error {
	return runSubcmd(appver.New(), output, true)
}

func topLevelCommand(args []string) (string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", false
	}

	return args[0], true
}
