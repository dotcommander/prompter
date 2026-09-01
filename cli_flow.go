package main

import (
	"flag"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
)

const (
	commandApply       = "apply"
	commandBrowse      = "browse"
	commandConfigAlias = "config"
	commandConfigure   = "configure"
	commandCritique    = "critique"
	commandImage       = "image"
	commandModels      = "models"
	commandRefine      = "refine"
	commandRewrite     = "rewrite"
)

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
		name, _, hasInlineValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		takesValue := false
		if registered := fs.Lookup(name); registered != nil {
			boolFlag, isBool := registered.Value.(interface{ IsBoolFlag() bool })
			takesValue = !isBool || !boolFlag.IsBoolFlag()
		}
		if takesValue && !hasInlineValue {
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

func resolveCommandSystemPrompt(f *flags, cfg *config.Config) error {
	switch {
	case f.command == commandImage || f.command == commandBrowse || f.command == commandConfigure || f.command == commandModels:
		return nil
	case f.command == commandApply:
		validation, err := loadCatalogSystemPrompt(cfg, f.promptName)
		if err != nil {
			return err
		}
		f.outputValidation = validation
		return nil
	case f.command == commandCritique:
		cfg.SystemPrompt = defaultCritiquePrompt
		return nil
	case f.command == commandRewrite:
		prompt, err := resolveRewritePrompt(f.rewriteMode)
		if err != nil {
			return err
		}
		cfg.SystemPrompt = prompt
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
