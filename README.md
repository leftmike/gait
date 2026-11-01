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

gait reads the first HCL config file found from `./gait.hcl`,
`~/.gait/gait.hcl`, or `~/.gait.hcl` (use `-config` to point at a specific
file, or `-no-config` to skip it).

```hcl
provider = "anthropic"          # default provider

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

## Sandboxes

Tools run commands and access files through a sandbox. Select one with
`sandbox = "<name>"` in the config or `-sandbox <name>`; `-sandbox none` runs
without one.

```hcl
sandbox = "dev"

sandbox "dev" {
    type  = "nono"                  # backend: none or nono
    read  = ["~/go/pkg/mod"]        # read-only
    write = ["/tmp"]                # read-write
    deny  = ["~/.aws"]              # no access
    env   = ["PATH", "HOME"]        # environment variables passed through

    network {
        mode  = "allow"             # none, all, or allow
        hosts = ["proxy.golang.org"]
    }
}
```

The working directory is read-write unless it is listed. Relative paths are
relative to the working directory. The network policy applies to commands
only; the default is `none`. A sandbox block may also hold a block named after
its backend for that backend's own settings.

The `none` backend does not restrict anything.

The `nono` backend runs each command under [nono](https://nono.sh), which must
be installed, and checks the file tools against the same policy. It grants
nothing beyond the policy except system paths needed to run programs (such as
`/usr`), so add `/tmp` to `write` if commands need temporary files. On Linux, a
path cannot be given less access than a directory containing it, such as
`deny = ["./.env"]` with the working directory writable. A host allow list
does not work on WSL2. Its block:

```hcl
    nono {
        binary = "/usr/local/bin/nono" # default: nono on PATH
    }
```
