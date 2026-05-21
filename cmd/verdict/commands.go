package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/appver"
	"github.com/KEINOS/go-verdict/cmd/verdict/internal/skill"
)

const (
	commandSkill   = "skill"
	commandVersion = "version"
)

func runTopLevelCommand(args []string, output io.Writer) (bool, error) {
	command, hasCommand := topLevelCommand(args)
	if !hasCommand {
		return false, nil
	}

	if command != commandSkill && command != commandVersion {
		return true, fmt.Errorf("%w: %s", errUnknownCommand, command)
	}

	if len(args) != 1 {
		return true, errUnexpectedCommandArgs
	}

	var subCommand subcmd

	if command == commandSkill {
		subCommand = skill.New()
	} else {
		subCommand = appver.New()
	}

	return true, runSubcmd(subCommand, output, command == commandVersion)
}

type subcmd interface {
	Run() (string, error)
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
