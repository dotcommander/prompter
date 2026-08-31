package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/dotcommander/prompter/internal/config"
)

const (
	commandAssemble  = "assemble"
	commandCritique  = "critique"
	commandEnhance   = "enhance"
	commandRun       = "run"
	commandRewrite   = "rewrite"
	commandStats     = "stats"
	commandFind      = "find"
	commandSearch    = "search"
	commandBrowse    = "browse"
	commandConfig    = "config"
	commandStyles    = "styles"
	commandModes     = "modes"
	commandProviders = "providers"
	commandUpdate    = "update"
	commandVersion   = "version"
	commandInit      = "init"
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
	case f.command == commandAssemble || f.command == commandStats ||
		f.command == commandFind || f.command == commandSearch || f.command == commandBrowse ||
		f.command == commandConfig || f.command == commandStyles || f.command == commandModes ||
		f.command == commandProviders || f.command == commandUpdate || f.command == commandVersion ||
		f.command == commandInit:
		return nil
	case f.command == commandRun:
		if f.styleSet || f.modeSet {
			return fmt.Errorf("run does not accept --style or --mode")
		}
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
		mode := f.rewriteMode
		if f.styleSet {
			if f.modeSet && f.style != mode {
				return fmt.Errorf("--style and --mode conflict: %q != %q", f.style, mode)
			}
			mode = f.style
		}
		prompt, err := resolveRewritePrompt(mode)
		if err != nil {
			return err
		}
		cfg.SystemPrompt = prompt
		return nil
	case f.styleSet || f.modeSet:
		style := f.style
		if f.modeSet {
			if f.styleSet && f.style != f.rewriteMode {
				return fmt.Errorf("--style and --mode conflict: %q != %q", f.style, f.rewriteMode)
			}
			style = f.rewriteMode
		}
		prompt, err := resolveStyle(style)
		if err != nil {
			return err
		}
		cfg.SystemPrompt = prompt
		return nil
	default:
		return loadSystemPrompt(cfg)
	}
}
