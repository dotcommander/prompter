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

type executionStatus struct {
	code int
	err  error
}

// executionPipeline owns the ordered process lifecycle. Commands retain their
// existing configuration, provider, input, and output behavior.
type executionPipeline struct {
	args   []string
	stdout io.Writer
	stderr io.Writer

	flags      *flags
	cfg        *config.Config
	runCommand func(context.Context, *flags, *config.Config, *slog.Logger) error
}

func newExecutionPipeline(args []string, stdout, stderr io.Writer) *executionPipeline {
	return &executionPipeline{args: args, stdout: stdout, stderr: stderr, runCommand: run}
}

func main() {
	if code := execute(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func execute(args []string, stdout, stderr io.Writer) int {
	return newExecutionPipeline(args, stdout, stderr).run()
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
		return commandStatus(showFinder(p.cfg))
	case commandConfigure:
		if !isInteractiveTerminal(os.Stdin, term.IsTerminal) || !isInteractiveTerminal(os.Stdout, term.IsTerminal) {
			printConfig(p.stdout, p.cfg)
			return &executionStatus{}
		}
		service, err := newModelCatalogService()
		if err != nil {
			return commandStatus(err)
		}
		catalog, _, err := service.loadOrFetch(context.Background(), p.cfg)
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
		catalog, err := service.refresh(context.Background(), p.cfg)
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
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	done := make(chan struct{})
	interruptDone := make(chan struct{})
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		defer close(interruptDone)
		select {
		case <-sigChan:
			cancel()
			fmt.Fprintf(p.stderr, "\r\033[K")
		case <-done:
		}
	}()

	err := p.runCommand(ctx, p.flags, p.cfg, newLoggerTo(p.stderr, p.flags.verbose))
	wasCanceled := ctx.Err() != nil
	signal.Stop(sigChan)
	close(done)
	<-interruptDone
	cancel()
	if err == nil {
		return &executionStatus{}
	}
	if wasCanceled {
		return &executionStatus{code: 130}
	}
	return &executionStatus{code: 1, err: err}
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
