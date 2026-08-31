# Troubleshooting

Purpose: recover quickly from common setup and runtime failures.

## Key and credential failures

Error examples:

- `cerebras API key not set`
- `openai API key not set`

Fix:

1. Set the provider's standard environment variable in your shell:
   ```bash
   export CEREBRAS_API_KEY="csk-..."
   # or for OpenAI:
   export OPENAI_API_KEY="sk-..."
   ```
   Alternatively, you can configure it via `PROMPTER_CEREBRAS_API_KEY` or `~/.config/prompter/config.json`.
2. For Gemini, provide an AI Studio key or authenticate Google ADC:
   ```bash
   # Option A: Google AI Studio key
   export GEMINI_API_KEY="AIza..."

   # Option B: Google Cloud Application Default Credentials
   gcloud auth application-default login
   ```
3. For local loopback providers (`wormhole` or `omlx`), no API key is required.
4. Run `prompter refine "test"` again.

## Provider and model failures

Error examples:

- `unknown provider "..."`
- `model not found`
- API `timeout` or `connection refused`
- `groq timed out after 1m0s`
- `wormhole: connection refused`
- `wormhole: unknown provider prefix`

Fix:

For timeouts, increase timeout with `PROMPTER_TIMEOUT=120` or `--dry-run` to inspect.

Default is 60s (streaming uses at least 180s). For other provider errors:

```bash
# Try a known provider
prompter refine -p openai "test"

# Override model
prompter refine -m gpt-4o "test"

# Override endpoint
prompter refine --base-url https://api.example.com "test"
```

For `wormhole`, `--base-url` is the proxy's OpenAI-compatible `/v1` URL:

```bash
prompter refine -p wormhole --base-url http://127.0.0.1:8080/v1 "test"
```

Use a `provider/model` identifier when the request must route to a specific
Wormhole upstream, for example `groq/openai/gpt-oss-120b`.

## Finder failures

If `prompter browse` does not show usable results:

1. Verify `prompts_dir` exists.
2. Verify it contains `.md` prompt files.
3. Run `prompter browse` again.

See `finder.md` and `prompt-files.md` for finder and file rules.

## Debugging checklist

- Run `prompter refine -v "test"` and inspect stderr
- Confirm config JSON is valid
- Confirm API key is valid for selected provider
- For `wormhole`, check `http://127.0.0.1:8080/health` and `/v1/models`
- Try another provider to isolate provider-specific failures

## Related docs

- `setup.md`
- `finder.md`
- `flags.md`
- `use-json-output.md`
