# Prompter documentation

Use these guides to run Prompter from the command line, keep reusable prompts locally, or contribute safely. For a credential-free first check from a checkout, run `GOWORK=off go run . image "desert observatory" --profile minimal`; it prints an assembled image prompt to standard output and does not generate an image.

| Your task | Guide | Start here when… |
| --- | --- | --- |
| Build from a checkout and configure remote prompting | [Setup](setup.md) | You need the offline first check or a configured provider. |
| Choose a command and its flags | [CLI flags](flags.md) | You need accepted inputs, outputs, or defaults. |
| Build a shell pipeline | [Common tasks](common-tasks.md) | You need stdin, stdout, or exit-status behavior. |
| Build and use a local prompt vault | [Prompt files](prompt-files.md) | You want `apply`, `browse`, or starter-prompt maintenance. |
| Use Prompter in automation | [Automation](use-json-output.md) | Your caller must handle standard streams, exit status, or image JSON. |
| Select a provider | [Providers](providers.md) | You need provider-specific configuration or local OMLX behavior. |
| Recover from a failure | [Troubleshooting](troubleshooting.md) | A command, credential, model, or finder is failing. |
| Change the repository | [Contributor guide](change-prompter.md) | You are modifying implementation or documentation. |

## Limits before choosing a route

- `image` is offline. Remote `refine`, `critique`, `rewrite`, and `apply` require a resolved provider.
- `browse` requires interactive standard input and standard error; do not use it for unattended automation.
- A streamed provider response can write partial text before a nonzero exit. Automation must discard that output.
- Validated catalog prompts reject `--stream` because validation needs buffered output.

Use `prompter <command> --help` to inspect options accepted by one command.