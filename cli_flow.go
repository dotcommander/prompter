package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
)

const (
	commandRefine = "refine"
	commandImage  = "image"
	commandConfig = "config"

	imageOperationFlag  = "--image"
	configOperationFlag = "--config"
)

// retiredCommands were operations in earlier releases. When one appears as the
// first argument the CLI returns a migration error instead of silently
// enriching the text with a remote provider call.
var retiredCommands = map[string]struct{}{
	"critique":  {},
	"rewrite":   {},
	"apply":     {},
	"browse":    {},
	"image":     {},
	"configure": {},
	"config":    {},
	"models":    {},
	"prompts":   {},
}

func isRetiredCommand(value string) bool {
	_, ok := retiredCommands[value]
	return ok
}

func retiredCommandError(name string) error {
	switch name {
	case commandImage:
		return fmt.Errorf("command %q was removed; build image prompts with %s", name, imageOperationFlag)
	case "configure", "config":
		return fmt.Errorf("command %q was removed; configure prompter with %s", name, configOperationFlag)
	default:
		return fmt.Errorf("command %q was removed; enrichment is the default operation or the explicit %q command, image prompts use %s, and configuration uses %s",
			name, commandRefine, imageOperationFlag, configOperationFlag)
	}
}

// consumesFollowingToken reports whether arg names a registered flag whose
// value arrives as the next separate token, mirroring flag package parsing.
func consumesFollowingToken(fs *flag.FlagSet, arg string) bool {
	name, _, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
	if inline {
		return false
	}
	registered := fs.Lookup(name)
	if registered == nil {
		return false
	}
	boolFlag, isBool := registered.Value.(interface{ IsBoolFlag() bool })
	return !isBool || !boolFlag.IsBoolFlag()
}

// interspersedFlagArgs moves recognized flag tokens before positional input so
// flag.FlagSet can accept documented forms such as "subject --count 3". Tokens
// after -- remain literal positional input.
func interspersedFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	literal := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if literal {
			positional = append(positional, arg)
			continue
		}
		if arg == "--" {
			literal = true
			continue
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}

		flags = append(flags, arg)
		if consumesFollowingToken(fs, arg) {
			if i+1 >= len(args) {
				return flags
			}
			i++
			flags = append(flags, args[i])
		}
	}

	if len(positional) == 0 {
		return flags
	}
	return append(append(flags, "--"), positional...)
}

// containsControlFlag ignores registered flag values and literal input.
func containsControlFlag(fs *flag.FlagSet, args []string, token string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return false
		}
		if arg == token {
			return true
		}
		if consumesFollowingToken(fs, arg) {
			i++
		}
	}
	return false
}

func operationFlagSet(command string) *flag.FlagSet {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	registerFlags(fs, &flags{command: command})
	return fs
}

// selectOperation resolves the single operation for an invocation and returns
// the remaining arguments for that operation's flag set. Only one operation per
// invocation is allowed; retired command words in the first position return the
// migration error so old scripts fail loudly instead of calling a provider.
func selectOperation(args []string) (string, []string, error) {
	if len(args) == 0 {
		return commandRefine, nil, nil
	}
	if isRetiredCommand(args[0]) {
		return "", nil, retiredCommandError(args[0])
	}
	switch args[0] {
	case commandRefine:
		rest := args[1:]
		fs := operationFlagSet(commandRefine)
		if containsControlFlag(fs, rest, imageOperationFlag) {
			return "", nil, fmt.Errorf("%s cannot be combined with %s", commandRefine, imageOperationFlag)
		}
		if containsControlFlag(fs, rest, configOperationFlag) {
			return "", nil, fmt.Errorf("%s cannot be combined with %s", commandRefine, configOperationFlag)
		}
		return commandRefine, rest, nil
	case imageOperationFlag:
		rest := args[1:]
		if containsControlFlag(operationFlagSet(commandImage), rest, configOperationFlag) {
			return "", nil, fmt.Errorf("%s and %s cannot be combined", imageOperationFlag, configOperationFlag)
		}
		if len(rest) > 0 && rest[0] == commandRefine {
			return "", nil, fmt.Errorf("%s cannot be combined with %q; use %q to pass literal input", imageOperationFlag, commandRefine, "--")
		}
		return commandImage, rest, nil
	case configOperationFlag:
		rest := args[1:]
		if containsControlFlag(operationFlagSet(commandConfig), rest, imageOperationFlag) {
			return "", nil, fmt.Errorf("%s and %s cannot be combined", imageOperationFlag, configOperationFlag)
		}
		if len(rest) > 0 && rest[0] == commandRefine {
			return "", nil, fmt.Errorf("%s cannot be combined with %q; use %q to pass literal input", configOperationFlag, commandRefine, "--")
		}
		return commandConfig, rest, nil
	case "--help", "-h", "--version", "-V":
		return "", nil, fmt.Errorf("%s cannot be combined with other arguments", args[0])
	default:
		// Refine-owned flags before the input text select enrichment by default.
		return commandRefine, args, nil
	}
}

func resolveCommandSystemPrompt(f *flags, cfg *config.Config) error {
	switch {
	case f.command == commandImage || f.command == commandConfig:
		return nil
	case f.command == commandRefine && f.styleSet:
		prompt, err := resolveStyle(f.style)
		if err != nil {
			return err
		}
		cfg.SystemPrompt = prompt
		return nil
	default:
		return loadSystemPrompt(cfg)
	}
}
