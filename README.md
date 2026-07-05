# gait
A simple tool for interacting with LLMs.

## Providers

gait supports the following model providers:

| Provider    | Flag          | Type   |
|-------------|---------------|--------|
| OpenAI      | `-openai`     | cloud  |
| Anthropic   | `-anthropic`  | cloud  |
| Google      | `-google`     | cloud  |
| Ollama      | `-ollama`     | local  |
| llama.cpp   | `-llamacpp`   | local  |

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
gait -anthropic -model claude-haiku-4-5-20251001
gait -ollama -model qwen2.5 -baseurl http://localhost:11545/v1
```
