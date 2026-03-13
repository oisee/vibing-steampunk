# CLI Coding Agents + VSP (SAP ADT MCP Server)

Інструкція з налаштування CLI-асистентів для роботи з SAP через [VSP (vibing-steampunk)](https://github.com/oisee/vibing-steampunk).

**VSP** — MCP-сервер, який надає AI-асистентам доступ до SAP ADT API: читання/запис коду, налагодження, тести, транспорти тощо.

**Переклади:** [English](README.md) | [Русский](README_RU.md) | [Español](README_ES.md)

---

## Зведена таблиця

| Інструмент | LLM | Безкоштовно? | MCP | Встановлення | Конфіг VSP |
|---|---|---|---|---|---|
| **Gemini CLI** | Gemini 2.5 Pro/Flash, 3 Pro | **Так** (1000 req/day) | Так | `npm i -g @google/gemini-cli` | `.gemini/settings.json` |
| **Claude Code** | Claude Opus/Sonnet 4.6 | Ні ($20+/міс) | Так | `curl -fsSL https://claude.ai/install.sh \| bash` | `.mcp.json` |
| **GitHub Copilot** | Claude, GPT-5, Gemini | Ні ($10+/міс) | Так | `npm i -g @github/copilot` | `.copilot/mcp-config.json` |
| **OpenAI Codex** | GPT-5-Codex, GPT-4.1 | Ні ($20+/міс) | Так | `npm i -g @openai/codex` | `.mcp.json` |
| **Qwen Code** | Qwen3-Coder | **Так** (1000 req/day) | Так | `npm i -g @qwen-code/qwen-code` | `.qwen/settings.json` |
| **OpenCode** | 75+ моделей (BYOK) | **Так** (свій ключ) | Так | `brew install anomalyco/tap/opencode` | `opencode.json` |
| **Goose** | 75+ провайдерів (BYOK) | **Так** (свій ключ) | Так | `brew install block-goose-cli` | `~/.config/goose/config.yaml` |
| **Mistral Vibe** | Devstral 2, локальні | Ні (API) / **Так** (Ollama) | Так | `pip install mistral-vibe` | `.vibe/config.toml` |

> **BYOK** = Bring Your Own Key (принеси свій API-ключ)

---

## 1. Gemini CLI (Google)

**Найкращий безкоштовний варіант.** 1000 запитів/день безкоштовно з обліковим записом Google.

### Встановлення

```bash
npm install -g @google/gemini-cli
# або без встановлення:
npx @google/gemini-cli
```

### Перший запуск

```bash
cd /path/to/your/project
gemini
# При першому запуску — увійти через Google акаунт
```

### Налаштування VSP

Створити файл `.gemini/settings.json` у теці проєкту:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<пароль>"
      }
    }
  }
}
```

### Перевірка MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Посилання
- GitHub: https://github.com/google-gemini/gemini-cli
- Документація: https://ai.google.dev/gemini-api/docs

---

## 2. Claude Code (Anthropic)

Творець стандарту MCP. Найглибша інтеграція з MCP-серверами.

### Встановлення

```bash
curl -fsSL https://claude.ai/install.sh | bash
# або:
brew install claude-code
```

### Перший запуск

```bash
cd /path/to/your/project
claude
# Потрібен обліковий запис Claude Pro ($20/міс) або API-ключ Anthropic
```

### Налаштування VSP

Створити файл `.mcp.json` у корені проєкту:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<пароль>"
      }
    }
  }
}
```

### Перевірка MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Посилання
- GitHub: https://github.com/anthropics/claude-code
- Документація: https://docs.anthropic.com/en/docs/claude-code

---

## 3. GitHub Copilot CLI

Мульти-модельний: Claude, GPT-5, Gemini — перемикання між моделями на льоту.

### Встановлення

```bash
npm install -g @github/copilot
# або через GitHub CLI:
gh extension install github/gh-copilot
```

### Перший запуск

```bash
cd /path/to/your/project
github-copilot
# Потрібна підписка GitHub Copilot ($10+/міс)
```

### Налаштування VSP

Створити файл `.copilot/mcp-config.json` у теці проєкту:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<пароль>"
      }
    }
  }
}
```

### Перевірка MCP

```
> Use the sap-adt tools to search for objects starting with ZCL_VDB
```

### Посилання
- GitHub: https://github.com/github/copilot-cli
- Документація: https://docs.github.com/en/copilot

---

## 4. OpenAI Codex CLI

### Встановлення

```bash
npm install -g @openai/codex
# або:
brew install --cask codex
```

### Перший запуск

```bash
cd /path/to/your/project
codex
# Потрібен ChatGPT Plus ($20/міс) або API-ключ OpenAI
```

### Налаштування VSP

Створити файл `.mcp.json` у корені проєкту (формат як у Claude Code):

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<пароль>"
      }
    }
  }
}
```

### Посилання
- GitHub: https://github.com/openai/codex

---

## 5. Qwen Code CLI (Alibaba)

**Безкоштовний.** 1000 запитів/день через Qwen OAuth.

### Встановлення

```bash
npm install -g @qwen-code/qwen-code@latest
# або:
brew install qwen-code
```

### Перший запуск

```bash
cd /path/to/your/project
qwen-code
# При першому запуску — вхід через Qwen OAuth (безкоштовно)
```

### Налаштування VSP

Створити файл `.qwen/settings.json` у теці проєкту:

```json
{
  "mcpServers": {
    "sap-adt": {
      "command": "/path/to/vsp-darwin-arm64",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "YOUR_USER",
        "SAP_PASSWORD": "<пароль>"
      },
      "timeout": 60000,
      "trust": false
    }
  }
}
```

### Посилання
- GitHub: https://github.com/QwenLM/qwen-code
- MCP Docs: https://qwenlm.github.io/qwen-code-docs/en/developers/tools/mcp-server/

---

## 6. OpenCode CLI

**Безкоштовний.** 75+ моделей, працює з будь-яким провайдером (Anthropic, OpenAI, Google, Ollama...).

### Встановлення

```bash
brew install anomalyco/tap/opencode
# або:
npm i -g opencode-ai@latest
# або:
curl -fsSL https://opencode.ai/install | bash
```

### Перший запуск

```bash
cd /path/to/your/project
opencode
# Вказати API-ключ провайдера (або підключити GitHub Copilot)
```

### Налаштування VSP

Створити файл `opencode.json` у корені проєкту:

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
        "SAP_PASSWORD": "<пароль>"
      },
      "timeout": 60000
    }
  }
}
```

> **Примітка:** Провайдер можна замінити на будь-який інший (Anthropic, OpenAI, Google, Ollama тощо).

### Посилання
- GitHub: https://github.com/opencode-ai/opencode
- MCP Docs: https://opencode.ai/docs/mcp-servers/

---

## 7. Goose (Block / Linux Foundation)

**Безкоштовний.** 75+ провайдерів, написаний на Rust. MCP — основа архітектури.

### Встановлення

```bash
brew install block-goose-cli
# або:
curl -fsSL https://github.com/block/goose/releases/download/stable/download_cli.sh | bash
```

### Перший запуск

```bash
goose configure
# Обрати провайдера (Azure, Anthropic, OpenAI, Google, Ollama...)
# Вказати API-ключ
goose
```

### Налаштування VSP

Скопіювати конфіг у `~/.config/goose/config.yaml`:

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
      SAP_PASSWORD: "<пароль>"
```

### Або додати через CLI

```bash
goose configure
# → Add extension → stdio → вказати шлях до vsp та змінні оточення
```

### Перевірка

```bash
goose info -v
```

### Посилання
- GitHub: https://github.com/block/goose
- Документація: https://block.github.io/goose/docs/guides/config-files

---

## 8. Mistral Vibe CLI

Підтримує **локальні моделі** через Ollama (безкоштовно).

### Встановлення

```bash
pip install mistral-vibe
# або:
brew install mistral-vibe
```

### Перший запуск

```bash
cd /path/to/your/project
vibe
# Потрібен API-ключ Mistral або налаштований Ollama
```

### Налаштування VSP

Створити файл `.vibe/config.toml` у теці проєкту:

```toml
# Провайдер (Ollama для безкоштовних локальних моделей)
[[providers]]
name = "ollama"
api_base = "http://localhost:11434/v1"
api_key_env_var = "OLLAMA_API_KEY"
api_style = "openai"
backend = "generic"

# Моделі
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

# MCP сервер VSP
[[mcp_servers]]
name = "sap-adt"
transport = "stdio"
command = "/path/to/vsp-darwin-arm64"
```

Створити `.vibe/.env`:
```bash
OLLAMA_API_KEY=not-required
SAP_URL=https://your-sap-host:44300
SAP_USER=YOUR_USER
SAP_PASSWORD=<пароль>
```

### Посилання
- GitHub: https://github.com/mistralai/mistral-vibe

---

## Формати MCP-конфігів — шпаргалка

| Інструмент | Формат | Файл конфігу | Ключ MCP | Ключ env |
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

## Рекомендації

### Безкоштовні варіанти для роботи з VSP

1. **Gemini CLI** — найкращий безкоштовний. 1000 запитів/день, Gemini 2.5 Pro з контекстом 1M токенів
2. **Qwen Code** — 1000 запитів/день безкоштовно через Qwen OAuth
3. **Mistral Vibe + Ollama** — повністю безкоштовно з локальними моделями (потрібен потужний GPU/Mac)
4. **OpenCode / Goose** — безкоштовні CLI, але потрібен API-ключ якогось провайдера

### Найкраща якість

1. **Claude Code** (Opus 4.6) — творець MCP, найкраща інтеграція
2. **GitHub Copilot** (мульти-модель) — можна перемикатися між Claude/GPT/Gemini
3. **Gemini CLI** (Gemini 2.5 Pro) — потужна модель + безкоштовно

---

## Тестовий MCP-сервер

Для перевірки MCP-з'єднання без SAP можна використати echo-сервер:

```bash
python3 /path/to/mcp-echo-server.py
```

Приклад конфігу (Claude Code / Codex / Gemini):
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

## VSP — швидкий старт

```bash
# Завантажити бінарник
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-darwin-arm64
chmod +x vsp-darwin-arm64

# Або зібрати з вихідного коду
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk && make build
```

Змінні оточення:
```bash
export SAP_URL=https://your-sap-host:44300
export SAP_USER=your-username
export SAP_PASSWORD=your-password
export SAP_CLIENT=001          # за замовчуванням
export SAP_MODE=focused        # focused (48 інструментів) або expert (96)
```

Докладніше: [VSP README](https://github.com/oisee/vibing-steampunk) | [MCP Usage Guide](https://github.com/oisee/vibing-steampunk/blob/main/MCP_USAGE.md)
