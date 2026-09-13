# Prompter documentation

Use these guides to run Prompter from the command line, store reusable prompts locally, or change the repository. For a credential-free first check from a checkout, run `GOWORK=off go run . image "desert observatory" --profile minimal`; it prints an assembled image prompt to standard output. The command builds prompt text only—it does not generate an image.

| Your task | Guide | Start here when… |
| --- | --- | --- |
| Install and configure a provider | [Setup](setup.md) | You need a working remote prompt command. |
| Choose flags and command options | [CLI flags](flags.md) | You know the command but need its inputs or output behavior. |
| Build and use a local prompt vault | [Prompt files](prompt-files.md) | You want `apply`, `browse`, or starter-prompt maintenance. |
| Use Prompter in a script | [Automation](use-json-output.md) | Your caller must handle stdout, stderr, and exit status. |
| Select or configure a provider | [Providers](providers.md) | You need provider-specific configuration or a local OMLX endpoint. |
| Recover from a failure | [Troubleshooting](troubleshooting.md) | A command, credential, model, or finder is failing. |
| Change the repository | [Contributor guide](change-prompter.md) | You are modifying implementation or documentation. |

## Limits before choosing a route

- `image` is offline, but remote `refine`, `critique`, `rewrite`, and `apply` operations need a configured provider.
- `browse` requires an interactive terminal; do not use it for unattended automation.
- Streamed output can be partial when the command exits nonzero. Automation should discard captured output on failure.
- Validated catalog prompts reject `--stream` because streamed text cannot be recalled for validation.

Each topic links to its source-owned command or configuration behavior. Use `prompter <command> --help` for the options accepted by one command.
