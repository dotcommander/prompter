package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"

	"github.com/charmbracelet/x/term"
	"github.com/dotcommander/prompter/internal/config"
)

type executionStage func(*executionPipeline) *executionStatus

type commandOutput struct {
	stdout io.Writer
	stderr io.Writer
}

type executionStatus struct {
	code int
	err  error
}

// executionPipeline owns the ordered process lifecycle. Commands retain their
// existing configuration, provider, input, and output behavior.
type executionPipeline struct {
	ctx    context.Context
	args   []string
	stdout io.Writer
	stderr io.Writer

	flags      *flags
	cfg        *config.Config
	runCommand func(context.Context, *flags, *config.Config, *slog.Logger, commandOutput) error
}

func newExecutionPipeline(ctx context.Context, args []string, stdout, stderr io.Writer) *executionPipeline {
	return &executionPipeline{ctx: ctx, args: args, stdout: stdout, stderr: stderr, runCommand: runWithOutput}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if code := executeContext(ctx, os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func execute(args []string, stdout, stderr io.Writer) int {
	return executeContext(context.Background(), args, stdout, stderr)
}

func executeContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return newExecutionPipeline(ctx, args, stdout, stderr).run()
}

func (p *executionPipeline) run() int {
	for _, stage := range []executionStage{
		(*executionPipeline).metadata,
		(*executionPipeline).parse,
		(*executionPipeline).configure,
		(*executionPipeline).routeLocalCommand,
		(*executionPipeline).validate,
		(*executionPipeline).executeCommand,
	} {
		if status := stage(p); status != nil {
			if p.ctx.Err() != nil {
				return 130
			}
			if status.err != nil {
				fmt.Fprintf(p.stderr, "error: %v\n", status.err)
			}
			return status.code
		}
	}
	return 0
}

func (p *executionPipeline) metadata() *executionStatus {
	p.args = rootArgs(p.args, isStdinPiped())
	if len(p.args) == 0 {
		printUsageTo(p.stderr)
		return &executionStatus{}
	}
	if len(p.args) != 1 {
		return nil
	}
	switch p.args[0] {
	case "--version", "-V":
		printVersion(p.stdout)
		return &executionStatus{}
	case "--help", "-h":
		printUsageTo(p.stderr)
		return &executionStatus{}
	default:
		return nil
	}
}

func (p *executionPipeline) parse() *executionStatus {
	parsed, err := parseArgsTo(p.args, p.stderr)
	if errors.Is(err, flag.ErrHelp) {
		return &executionStatus{}
	}
	if err != nil {
		return &executionStatus{code: 2, err: err}
	}
	p.flags = parsed
	return nil
}

func (p *executionPipeline) configure() *executionStatus {
	cfg, err := config.Load()
	if err != nil {
		return &executionStatus{code: 1, err: err}
	}
	p.cfg = cfg
	return nil
}

func (p *executionPipeline) routeLocalCommand() *executionStatus {
	switch p.flags.command {
	case commandBrowse:
		if err := validateBrowseTerminal(
			isInteractiveTerminal(os.Stdin, term.IsTerminal),
			isInteractiveTerminal(os.Stderr, term.IsTerminal),
		); err != nil {
			return &executionStatus{code: 1, err: err}
		}
		return commandStatus(showFinder(p.cfg, commandOutput{stdout: p.stdout, stderr: p.stderr}))
	case commandConfigure:
		if !isInteractiveTerminal(os.Stdin, term.IsTerminal) || !isInteractiveTerminal(os.Stdout, term.IsTerminal) {
			printConfig(p.stdout, p.cfg)
			return &executionStatus{}
		}
		service, err := newModelCatalogService()
		if err != nil {
			return commandStatus(err)
		}
		catalog, _, err := service.loadOrFetch(p.ctx, p.cfg)
		if err != nil {
			useEmbedded, confirmErr := confirmEmbeddedModelCatalog(err)
			if confirmErr != nil {
				return commandStatus(confirmErr)
			}
			if !useEmbedded {
				return commandStatus(err)
			}
		}
		return commandStatus(RunConfigForm(p.cfg, catalogModelChoices(catalog)))
	case commandModels:
		service, err := newModelCatalogService()
		if err != nil {
			return commandStatus(err)
		}
		catalog, err := service.refresh(p.ctx, p.cfg)
		if err != nil {
			return commandStatus(err)
		}
		printModelCatalog(p.stdout, catalog)
		return &executionStatus{}
	case commandPrompts:
		return commandStatus(runPromptMaintenance(p.stdout, p.cfg.PromptsDir, p.flags.promptAction, p.flags.dryRun))
	default:
		return nil
	}
}

func (p *executionPipeline) validate() *executionStatus {
	if commandRequiresInput(p.flags.command) && len(p.flags.args) == 0 && p.flags.file == "" && !isStdinPiped() {
		return &executionStatus{code: 1, err: fmt.Errorf("%s requires input", p.flags.command)}
	}
	if err := resolveCommandSystemPrompt(p.flags, p.cfg); err != nil {
		return &executionStatus{code: 1, err: err}
	}
	return nil
}

func (p *executionPipeline) executeCommand() *executionStatus {
	err := p.runCommand(p.ctx, p.flags, p.cfg, newLoggerTo(p.stderr, p.flags.verbose), commandOutput{stdout: p.stdout, stderr: p.stderr})
	if p.ctx.Err() != nil {
		fmt.Fprint(p.stderr, "\r\033[K")
	}
	return commandStatus(err)
}

func commandStatus(err error) *executionStatus {
	if err != nil {
		return &executionStatus{code: 1, err: err}
	}
	return &executionStatus{}
}

func commandRequiresInput(command string) bool {
	switch command {
	case commandRefine, commandCritique, commandApply, commandRewrite, commandImage:
		return true
	default:
		return false
	}
}
