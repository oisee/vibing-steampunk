# CLI Coding Agents + VSP (SAP ADT MCP Server)

Guia para configurar asistentes CLI de programacion para trabajar con SAP a traves de [VSP (vibing-steampunk)](https://github.com/oisee/vibing-steampunk).

**VSP** es un servidor MCP que da a los asistentes de IA acceso a la API SAP ADT: lectura/escritura de codigo, depuracion, pruebas, transportes y mas.

**Traducciones:** [English](README.md) | [Русский](README_RU.md) | [Українська](README_UA.md)

---

## Tabla Resumen

| Herramienta | LLM | Gratis? | MCP | Instalacion | Config VSP |
|---|---|---|---|---|---|
| **Gemini CLI** | Gemini 2.5 Pro/Flash, 3 Pro | **Si** (1000 req/dia) | Si | `npm i -g @google/gemini-cli` | `.gemini/settings.json` |
| **Claude Code** | Claude Opus/Sonnet 4.6 | No ($20+/mes) | Si | `curl -fsSL https://claude.ai/install.sh \| bash` | `.mcp.json` |
| **GitHub Copilot** | Claude, GPT-5, Gemini | No ($10+/mes) | Si | `npm i -g @github/copilot` | `.copilot/mcp-config.json` |
| **OpenAI Codex** | GPT-5-Codex, GPT-4.1 | No ($20+/mes) | Si | `npm i -g @openai/codex` | `.mcp.json` |
| **Qwen Code** | Qwen3-Coder | **Si** (1000 req/dia) | Si | `npm i -g @qwen-code/qwen-code` | `.qwen/settings.json` |
| **OpenCode** | 75+ modelos (BYOK) | **Si** (tu clave) | Si | `brew install anomalyco/tap/opencode` | `opencode.json` |
| **Goose** | 75+ proveedores (BYOK) | **Si** (tu clave) | Si | `brew install block-goose-cli` | `~/.config/goose/config.yaml` |
| **Mistral Vibe** | Devstral 2, locales | No (API) / **Si** (Ollama) | Si | `pip install mistral-vibe` | `.vibe/config.toml` |

> **BYOK** = Bring Your Own Key (trae tu propia clave API)

---

## 1. Gemini CLI (Google)

**Mejor opcion gratuita.** 1000 solicitudes/dia gratis con cuenta de Google.

### Instalacion

```bash
npm install -g @google/gemini-cli
# o sin instalar:
npx @google/gemini-cli
```

### Primer inicio

```bash
cd /path/to/your/project
gemini
# En el primer inicio — iniciar sesion con cuenta de Google
```

### Configuracion de VSP

Crear archivo `.gemini/settings.json` en la carpeta del proyecto:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<contrasena>"
      }
    }
  }
}
```

### Verificar MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Enlaces
- GitHub: https://github.com/google-gemini/gemini-cli
- Documentacion: https://ai.google.dev/gemini-api/docs

---

## 2. Claude Code (Anthropic)

Creador del estandar MCP. La integracion MCP mas profunda.

### Instalacion

```bash
curl -fsSL https://claude.ai/install.sh | bash
# o:
brew install claude-code
```

### Primer inicio

```bash
cd /path/to/your/project
claude
# Requiere cuenta Claude Pro ($20/mes) o clave API de Anthropic
```

### Configuracion de VSP

Crear archivo `.mcp.json` en la raiz del proyecto:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<contrasena>"
      }
    }
  }
}
```

### Verificar MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Enlaces
- GitHub: https://github.com/anthropics/claude-code
- Documentacion: https://docs.anthropic.com/en/docs/claude-code

---

## 3. GitHub Copilot CLI

Multi-modelo: Claude, GPT-5, Gemini — cambiar entre modelos sobre la marcha.

### Instalacion

```bash
npm install -g @github/copilot
# o via GitHub CLI:
gh extension install github/gh-copilot
```

### Primer inicio

```bash
cd /path/to/your/project
github-copilot
# Requiere suscripcion a GitHub Copilot ($10+/mes)
```

### Configuracion de VSP

Crear archivo `.copilot/mcp-config.json` en la carpeta del proyecto:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<contrasena>"
      }
    }
  }
}
```

### Verificar MCP

```
> Use the sap-adt tools to search for objects starting with ZCL_VDB
```

### Enlaces
- GitHub: https://github.com/github/copilot-cli
- Documentacion: https://docs.github.com/en/copilot

---

## 4. OpenAI Codex CLI

### Instalacion

```bash
npm install -g @openai/codex
# o:
brew install --cask codex
```

### Primer inicio

```bash
cd /path/to/your/project
codex
# Requiere ChatGPT Plus ($20/mes) o clave API de OpenAI
```

### Configuracion de VSP

Crear archivo `.mcp.json` en la raiz del proyecto (mismo formato que Claude Code):

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<contrasena>"
      }
    }
  }
}
```

### Enlaces
- GitHub: https://github.com/openai/codex

---

## 5. Qwen Code CLI (Alibaba)

**Gratuito.** 1000 solicitudes/dia via Qwen OAuth.

### Instalacion

```bash
npm install -g @qwen-code/qwen-code@latest
# o:
brew install qwen-code
```

### Primer inicio

```bash
cd /path/to/your/project
qwen-code
# En el primer inicio — iniciar sesion via Qwen OAuth (gratis)
```

### Configuracion de VSP

Crear archivo `.qwen/settings.json` en la carpeta del proyecto:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<contrasena>"
      },
      "timeout": 60000,
      "trust": false
    }
  }
}
```

### Enlaces
- GitHub: https://github.com/QwenLM/qwen-code
- MCP Docs: https://qwenlm.github.io/qwen-code-docs/en/developers/tools/mcp-server/

---

## 6. OpenCode CLI

**Gratuito.** 75+ modelos, funciona con cualquier proveedor (Anthropic, OpenAI, Google, Ollama...).

### Instalacion

```bash
brew install anomalyco/tap/opencode
# o:
npm i -g opencode-ai@latest
# o:
curl -fsSL https://opencode.ai/install | bash
```

### Primer inicio

```bash
cd /path/to/your/project
opencode
# Introducir la clave API del proveedor (o conectar GitHub Copilot)
```

### Configuracion de VSP

Crear archivo `opencode.json` en la raiz del proyecto:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "azure-openai": {
      "options": {
        "apiKey": "{env:AZURE_OPENAI_API_KEY}",
        "resourceName": "your-resource",
        "apiVersion": "{env:AZURE_OPENAI_API_VERSION}"
      }
    }
  },
  "mcp": {
    "sap-adt": {
      "type": "local",
      "command": ["/path/to/vsp-darwin-arm64"],
      "enabled": true,
      "environment": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<contrasena>"
      },
      "timeout": 60000
    }
  }
}
```

> **Nota:** El proveedor se puede reemplazar por cualquier otro (Anthropic, OpenAI, Google, Ollama, etc.).

### Enlaces
- GitHub: https://github.com/opencode-ai/opencode
- MCP Docs: https://opencode.ai/docs/mcp-servers/

---

## 7. Goose (Block / Linux Foundation)

**Gratuito.** 75+ proveedores, escrito en Rust. MCP es un principio arquitectonico central.

### Instalacion

```bash
brew install block-goose-cli
# o:
curl -fsSL https://github.com/block/goose/releases/download/stable/download_cli.sh | bash
```

### Primer inicio

```bash
goose configure
# Elegir proveedor (Azure, Anthropic, OpenAI, Google, Ollama...)
# Introducir clave API
goose
```

### Configuracion de VSP

Copiar config a `~/.config/goose/config.yaml`:

```yaml
extensions:
  sap-adt:
    enabled: true
    name: sap-adt
    type: stdio
    cmd: "/path/to/vsp-darwin-arm64"
    args: []
    description: "SAP ABAP Development Tools via MCP"
    timeout: 300
    envs:
      SAP_URL: "https://your-sap-host:44300"
      SAP_USER: "YOUR_USER"
      SAP_PASSWORD: "<contrasena>"
```

### O agregar via CLI

```bash
goose configure
# → Add extension → stdio → introducir ruta a vsp y variables de entorno
```

### Verificar

```bash
goose info -v
```

### Enlaces
- GitHub: https://github.com/block/goose
- Documentacion: https://block.github.io/goose/docs/guides/config-files

---

## 8. Mistral Vibe CLI

Soporta **modelos locales** via Ollama (gratis).

### Instalacion

```bash
pip install mistral-vibe
# o:
brew install mistral-vibe
```

### Primer inicio

```bash
cd /path/to/your/project
vibe
# Requiere clave API de Mistral o Ollama configurado
```

### Configuracion de VSP

Crear archivo `.vibe/config.toml` en la carpeta del proyecto:

```toml
# Proveedor (Ollama para modelos locales gratuitos)
[[providers]]
name = "ollama"
api_base = "http://localhost:11434/v1"
api_key_env_var = "OLLAMA_API_KEY"
api_style = "openai"
backend = "generic"

# Modelos
[[models]]
name = "devstral-small-2:latest"
provider = "ollama"
alias = "devstral"
temperature = 0.2

[[models]]
name = "qwen2.5-coder:32b"
provider = "ollama"
alias = "qwen-coder"
temperature = 0.2

# Servidor MCP VSP
[[mcp_servers]]
name = "sap-adt"
transport = "stdio"
command = "/path/to/vsp-darwin-arm64"
```

Crear `.vibe/.env`:
```bash
OLLAMA_API_KEY=not-required
SAP_URL=https://your-sap-host:44300
SAP_USER=YOUR_USER
SAP_PASSWORD=<contrasena>
```

### Enlaces
- GitHub: https://github.com/mistralai/mistral-vibe

---

## Formatos de Config MCP — Hoja de referencia

| Herramienta | Formato | Archivo de config | Clave MCP | Clave env |
|---|---|---|---|---|
| Claude Code | JSON | `.mcp.json` | `mcpServers` | `env` |
| Gemini CLI | JSON | `.gemini/settings.json` | `mcpServers` | `env` |
| Copilot | JSON | `.copilot/mcp-config.json` | `mcpServers` | `env` |
| Codex | JSON | `.mcp.json` | `mcpServers` | `env` |
| Qwen Code | JSON | `.qwen/settings.json` | `mcpServers` | `env` |
| OpenCode | JSON | `opencode.json` | `mcp` | `environment` |
| Goose | YAML | `~/.config/goose/config.yaml` | `extensions` | `envs` |
| Mistral Vibe | TOML | `.vibe/config.toml` | `[[mcp_servers]]` | `.vibe/.env` |

---

## Recomendaciones

### Opciones gratuitas para trabajar con VSP

1. **Gemini CLI** — mejor opcion gratuita. 1000 solicitudes/dia, Gemini 2.5 Pro con contexto de 1M tokens
2. **Qwen Code** — 1000 solicitudes/dia gratis via Qwen OAuth
3. **Mistral Vibe + Ollama** — completamente gratis con modelos locales (necesita GPU potente/Mac)
4. **OpenCode / Goose** — CLIs gratuitos, pero necesitan clave API de algun proveedor

### Mejor calidad

1. **Claude Code** (Opus 4.6) — creador de MCP, mejor integracion
2. **GitHub Copilot** (multi-modelo) — cambiar entre Claude/GPT/Gemini
3. **Gemini CLI** (Gemini 2.5 Pro) — modelo potente + gratis

---

## Servidor MCP de prueba

Para verificar la conectividad MCP sin SAP, usar el servidor echo:

```bash
python3 /path/to/mcp-echo-server.py
```

Ejemplo de config (Claude Code / Codex / Gemini):
```json
{
  "mcpServers": {
    "echo": {
      "command": "python3",
      "args": ["/path/to/mcp-echo-server.py"]
    }
  }
}
```

---

## VSP — Inicio rapido

```bash
# Descargar binario
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-darwin-arm64
chmod +x vsp-darwin-arm64

# O compilar desde fuente
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk && make build
```

Variables de entorno:
```bash
export SAP_URL=https://your-sap-host:44300
export SAP_USER=your-username
export SAP_PASSWORD=your-password
export SAP_CLIENT=001          # por defecto
export SAP_MODE=focused        # focused (48 herramientas) o expert (96)
```

Mas informacion: [VSP README](https://github.com/oisee/vibing-steampunk) | [MCP Usage Guide](https://github.com/oisee/vibing-steampunk/blob/main/MCP_USAGE.md)
