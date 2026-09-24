# Prompter documentation

Use these guides to run Prompter from the command line or contribute safely. For a credential-free first check from a checkout, run `GOWORK=off go run . --image "desert observatory" --profile minimal`; it prints an assembled image prompt to standard output and does not generate an image.

| Your task | Guide | Start here when… |
| --- | --- | --- |
| Build from a checkout and configure remote prompting | [Setup](setup.md) | You need the offline first check or a configured provider. |
| Choose an operation and its flags | [CLI flags](flags.md) | You need accepted inputs, outputs, or defaults. |
| Build a shell pipeline | [Common tasks](common-tasks.md) | You need stdin, stdout, or exit-status behavior. |
| Use Prompter in automation | [Automation](use-json-output.md) | Your caller must handle standard streams, exit status, or image JSON. |
| Select a provider | [Providers](providers.md) | You need provider-specific configuration or local OMLX behavior. |
| Recover from a failure | [Troubleshooting](troubleshooting.md) | An operation, credential, or model is failing. |
| Change the repository | [Contributor guide](change-prompter.md) | You are modifying implementation or documentation. |

## Limits before choosing a route

- `--image` is offline. Enrichment (`refine` or the default form) resolves a provider and can contact it.
- `--config` opens its form only on interactive terminals; redirected output prints resolved settings without a network request.
- Only one operation runs per invocation. Retired command words (`critique`, `rewrite`, `apply`, `browse`, `models`, `prompts`, `image`, `configure`) return a migration error.

Use `prompter refine --help`, `prompter --image --help`, or `prompter --config --help` to inspect one operation.
