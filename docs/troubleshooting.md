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
2. For Gemini, authenticate Google ADC (the default Vertex AI endpoint) or
   switch to the Google AI Studio endpoint:
   ```bash
   # Option A: Google Cloud Application Default Credentials (default endpoint)
   gcloud auth application-default login
   export GOOGLE_CLOUD_PROJECT="your-project-id"

   # Option B: Google AI Studio — requires both the key and the AI Studio base URL
   export GEMINI_API_KEY="AIza..."
   # set gemini.base_url to https://generativelanguage.googleapis.com/v1beta
   ```
   An AIza key alone does not replace ADC on the default Vertex AI endpoint.
3. For the local loopback provider (`omlx`), no API key is required.
4. Run `prompter refine "test"` again.

## Provider and model failures

Error examples:

- `unknown provider "..."`
- `model not found`
- API `timeout` or `connection refused`
- `groq timed out after 1m0s`

Fix:

For timeouts, increase timeout with `PROMPTER_TIMEOUT=120` or `--dry-run` to inspect.

Default is 60s (streaming uses at least 180s). For other provider errors:

```bash
# Try a known provider
prompter refine -p openai "test"

# Override model
prompter refine -m gpt-5.6-luna "test"

# Override endpoint
prompter refine --base-url https://api.example.com "test"
```

## Finder failures

If `prompter browse` does not show usable results:

1. Run `prompter browse` again — an empty primary vault is auto-created and
   seeded with the eight starter prompts on first launch.
2. Verify `prompts_dir` / `prompts_dirs` exist and contain `.md` prompt files.
3. Confirm the terminal is interactive (`browse` refuses piped input).

See `finder.md` and `prompt-files.md` for finder and file rules.

## Debugging checklist

- Run `prompter refine -v "test"` and inspect stderr
- Confirm config JSON is valid
- Confirm API key is valid for selected provider
- For `omlx`, check `http://127.0.0.1:8000/v1/models`
- Try another provider to isolate provider-specific failures

## Related docs

- `setup.md`
- `finder.md`
- `flags.md`
- `use-json-output.md`
