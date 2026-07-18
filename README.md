# gait
A simple tool for interacting with LLMs.

## Providers

gait supports the following model providers:

| Provider     | Flag           | Type   |
|--------------|----------------|--------|
| Anthropic    | `-anthropic`   | cloud  |
| Google       | `-google`      | cloud  |
| Hugging Face | `-huggingface` | cloud  |
| llama.cpp    | `-llamacpp`    | local  |
| Ollama       | `-ollama`      | local  |
| OpenAI       | `-openai`      | cloud  |
| OpenRouter   | `-openrouter`  | cloud  |

Cloud providers read their API key from config (`api_key`) or from an
environment variable (`ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `HF_TOKEN`,
`OPENAI_API_KEY`, `OPENROUTER_API_KEY`).

## Configuration

gait reads an HCL config file from `~/.gait/gait.hcl`, `~/.gait.hcl`, or
`./gait.hcl` (use `-config` to point at a specific file, or `-no-config` to
skip it).

```hcl
provider "anthropic"            # default provider

provider "anthropic" {
    model   = "claude-opus-4-1"
    api_key = "sk-ant-..."
}

provider "ollama" {
    model    = "llama3.2:3b"
}
```

Examples:

```
gait -anthropic -model claude-sonnet-4-6
gait -ollama -model qwen2.5 -baseurl http://localhost:11545/v1
```
