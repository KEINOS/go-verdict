package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/appver"
	"github.com/KEINOS/go-verdict/cmd/verdict/internal/helptopic"
	"github.com/KEINOS/go-verdict/cmd/verdict/internal/hotspot"
	"github.com/KEINOS/go-verdict/cmd/verdict/internal/skill"
)

const (
	cmdHelp    = "help"
	cmdHotspot = "hotspot"
	cmdSkill   = "skill"
	cmdVersion = "version"
)

type subcmd interface {
	Run() (string, error)
}

// commandHandler runs one top-level command with its remaining args.
type commandHandler func(args []string, output io.Writer) error

// commandHandlers maps each top-level command name to its handler.
func commandHandlers() map[string]commandHandler {
	return map[string]commandHandler{
		cmdHelp:    runHelpCommand,
		cmdHotspot: runHotspotCommand,
		cmdSkill:   textSubcmdHandler(skill.New(), false),
		cmdVersion: textSubcmdHandler(appver.New(), true),
	}
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

	handler, ok := commandHandlers()[command]
	if !ok {
		return commandHandled, fmt.Errorf("%w: %s", errUnknownCommand, command)
	}

	return commandHandled, handler(args[1:], output)
}

func runHelpCommand(args []string, output io.Writer) error {
	switch len(args) {
	case 0:
		return writeString(output, helptopic.IndexText())
	case 1:
		text, err := helptopic.Text(args[0])
		if err != nil {
			return fmt.Errorf("running help command: %w", err)
		}

		return writeString(output, text)
	default:
		return errUnexpectedCommandArgs
	}
}

func runHotspotCommand(args []string, output io.Writer) error {
	err := hotspot.New().Run(args, output)
	if err != nil {
		return fmt.Errorf("running hotspot command: %w", err)
	}

	return nil
}

// textSubcmdHandler adapts a no-argument text subcommand to a commandHandler.
func textSubcmdHandler(command subcmd, addTrailingNewline bool) commandHandler {
	return func(args []string, output io.Writer) error {
		if len(args) != 0 {
			return errUnexpectedCommandArgs
		}

		return runSubcmd(command, output, addTrailingNewline)
	}
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
