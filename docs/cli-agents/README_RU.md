# CLI Coding Agents + VSP (SAP ADT MCP Server)

Инструкция по настройке CLI-ассистентов для работы с SAP через [VSP (vibing-steampunk)](https://github.com/oisee/vibing-steampunk).

**VSP** — MCP-сервер, который даёт AI-ассистентам доступ к SAP ADT API: чтение/запись кода, отладка, тесты, транспорты и т.д.

**Переводы:** [English](README.md) | [Українська](README_UA.md) | [Español](README_ES.md)

---

## Сводная таблица

| Инструмент | LLM | Бесплатно? | MCP | Установка | Конфиг VSP |
|---|---|---|---|---|---|
| **Gemini CLI** | Gemini 2.5 Pro/Flash, 3 Pro | **Да** (1000 req/day) | Да | `npm i -g @google/gemini-cli` | `.gemini/settings.json` |
| **Claude Code** | Claude Opus/Sonnet 4.6 | Нет ($20+/мес) | Да | `curl -fsSL https://claude.ai/install.sh \| bash` | `.mcp.json` |
| **GitHub Copilot** | Claude, GPT-5, Gemini | Нет ($10+/мес) | Да | `npm i -g @github/copilot` | `.copilot/mcp-config.json` |
| **OpenAI Codex** | GPT-5-Codex, GPT-4.1 | Нет ($20+/мес) | Да | `npm i -g @openai/codex` | `.mcp.json` |
| **Qwen Code** | Qwen3-Coder | **Да** (1000 req/day) | Да | `npm i -g @qwen-code/qwen-code` | `.qwen/settings.json` |
| **OpenCode** | 75+ моделей (BYOK) | **Да** (свой ключ) | Да | `brew install anomalyco/tap/opencode` | `opencode.json` |
| **Goose** | 75+ провайдеров (BYOK) | **Да** (свой ключ) | Да | `brew install block-goose-cli` | `~/.config/goose/config.yaml` |
| **Mistral Vibe** | Devstral 2, локальные | Нет (API) / **Да** (Ollama) | Да | `pip install mistral-vibe` | `.vibe/config.toml` |

> **BYOK** = Bring Your Own Key (принеси свой API-ключ)

---

## 1. Gemini CLI (Google)

**Лучший бесплатный вариант.** 1000 запросов/день бесплатно с аккаунтом Google.

### Установка

```bash
npm install -g @google/gemini-cli
# или без установки:
npx @google/gemini-cli
```

### Первый запуск

```bash
cd /path/to/your/project
gemini
# При первом запуске — войти через Google аккаунт
```

### Настройка VSP

Создать файл `.gemini/settings.json` в папке проекта:

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

### Проверка MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Ссылки
- GitHub: https://github.com/google-gemini/gemini-cli
- Документация: https://ai.google.dev/gemini-api/docs

---

## 2. Claude Code (Anthropic)

Создатель стандарта MCP. Самая глубокая интеграция с MCP-серверами.

### Установка

```bash
curl -fsSL https://claude.ai/install.sh | bash
# или:
brew install claude-code
```

### Первый запуск

```bash
cd /path/to/your/project
claude
# Нужен аккаунт Claude Pro ($20/мес) или API-ключ Anthropic
```

### Настройка VSP

Создать файл `.mcp.json` в корне проекта:

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

### Проверка MCP

```
> Use the SearchObject tool to find classes starting with ZCL_VDB
```

### Ссылки
- GitHub: https://github.com/anthropics/claude-code
- Документация: https://docs.anthropic.com/en/docs/claude-code

---

## 3. GitHub Copilot CLI

Мульти-модельный: Claude, GPT-5, Gemini — переключение между моделями на лету.

### Установка

```bash
npm install -g @github/copilot
# или через GitHub CLI:
gh extension install github/gh-copilot
```

### Первый запуск

```bash
cd /path/to/your/project
github-copilot
# Нужна подписка GitHub Copilot ($10+/мес)
```

### Настройка VSP

Создать файл `.copilot/mcp-config.json` в папке проекта:

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

### Проверка MCP

```
> Use the sap-adt tools to search for objects starting with ZCL_VDB
```

### Ссылки
- GitHub: https://github.com/github/copilot-cli
- Документация: https://docs.github.com/en/copilot

---

## 4. OpenAI Codex CLI

### Установка

```bash
npm install -g @openai/codex
# или:
brew install --cask codex
```

### Первый запуск

```bash
cd /path/to/your/project
codex
# Нужен ChatGPT Plus ($20/мес) или API-ключ OpenAI
```

### Настройка VSP

Создать файл `.mcp.json` в корне проекта (формат как у Claude Code):

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

### Ссылки
- GitHub: https://github.com/openai/codex

---

## 5. Qwen Code CLI (Alibaba)

**Бесплатный.** 1000 запросов/день через Qwen OAuth.

### Установка

```bash
npm install -g @qwen-code/qwen-code@latest
# или:
brew install qwen-code
```

### Первый запуск

```bash
cd /path/to/your/project
qwen-code
# При первом запуске — вход через Qwen OAuth (бесплатно)
```

### Настройка VSP

Создать файл `.qwen/settings.json` в папке проекта:

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

### Ссылки
- GitHub: https://github.com/QwenLM/qwen-code
- MCP Docs: https://qwenlm.github.io/qwen-code-docs/en/developers/tools/mcp-server/

---

## 6. OpenCode CLI

**Бесплатный.** 75+ моделей, работает с любым провайдером (Anthropic, OpenAI, Google, Ollama...).

### Установка

```bash
brew install anomalyco/tap/opencode
# или:
npm i -g opencode-ai@latest
# или:
curl -fsSL https://opencode.ai/install | bash
```

### Первый запуск

```bash
cd /path/to/your/project
opencode
# Указать API-ключ провайдера (или подключить GitHub Copilot)
```

### Настройка VSP

Создать файл `opencode.json` в корне проекта:

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

> **Заметка:** Провайдер можно заменить на любой другой (Anthropic, OpenAI, Google, Ollama и т.д.).

### Ссылки
- GitHub: https://github.com/opencode-ai/opencode
- MCP Docs: https://opencode.ai/docs/mcp-servers/

---

## 7. Goose (Block / Linux Foundation)

**Бесплатный.** 75+ провайдеров, написан на Rust. MCP — основа архитектуры.

### Установка

```bash
brew install block-goose-cli
# или:
curl -fsSL https://github.com/block/goose/releases/download/stable/download_cli.sh | bash
```

### Первый запуск

```bash
goose configure
# Выбрать провайдера (Azure, Anthropic, OpenAI, Google, Ollama...)
# Указать API-ключ
goose
```

### Настройка VSP

Скопировать конфиг в `~/.config/goose/config.yaml`:

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

### Или добавить через CLI

```bash
goose configure
# → Add extension → stdio → указать путь к vsp и переменные окружения
```

### Проверка

```bash
goose info -v
```

### Ссылки
- GitHub: https://github.com/block/goose
- Документация: https://block.github.io/goose/docs/guides/config-files

---

## 8. Mistral Vibe CLI

Поддерживает **локальные модели** через Ollama (бесплатно).

### Установка

```bash
pip install mistral-vibe
# или:
brew install mistral-vibe
```

### Первый запуск

```bash
cd /path/to/your/project
vibe
# Нужен API-ключ Mistral или настроенный Ollama
```

### Настройка VSP

Создать файл `.vibe/config.toml` в папке проекта:

```toml
# Провайдер (Ollama для бесплатных локальных моделей)
[[providers]]
name = "ollama"
api_base = "http://localhost:11434/v1"
api_key_env_var = "OLLAMA_API_KEY"
api_style = "openai"
backend = "generic"

# Модели
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

Создать `.vibe/.env`:
```bash
OLLAMA_API_KEY=not-required
SAP_URL=https://your-sap-host:44300
SAP_USER=YOUR_USER
SAP_PASSWORD=<пароль>
```

### Ссылки
- GitHub: https://github.com/mistralai/mistral-vibe

---

## Форматы MCP-конфигов — шпаргалка

| Инструмент | Формат | Файл конфига | Ключ MCP | Ключ env |
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

## Рекомендации

### Бесплатные варианты для работы с VSP

1. **Gemini CLI** — лучший бесплатный. 1000 запросов/день, Gemini 2.5 Pro с контекстом 1M токенов
2. **Qwen Code** — 1000 запросов/день бесплатно через Qwen OAuth
3. **Mistral Vibe + Ollama** — полностью бесплатно с локальными моделями (нужен мощный GPU/Mac)
4. **OpenCode / Goose** — бесплатные CLI, но нужен API-ключ какого-то провайдера

### Лучшее качество

1. **Claude Code** (Opus 4.6) — создатель MCP, лучшая интеграция
2. **GitHub Copilot** (мульти-модель) — можно переключаться между Claude/GPT/Gemini
3. **Gemini CLI** (Gemini 2.5 Pro) — сильная модель + бесплатно

---

## Тестовый MCP-сервер

Для проверки MCP-подключения без SAP можно использовать echo-сервер:

```bash
python3 /path/to/mcp-echo-server.py
```

Пример конфига (Claude Code / Codex / Gemini):
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

## VSP — быстрый старт

```bash
# Скачать бинарник
curl -LO https://github.com/oisee/vibing-steampunk/releases/latest/download/vsp-darwin-arm64
chmod +x vsp-darwin-arm64

# Или собрать из исходников
git clone https://github.com/oisee/vibing-steampunk.git
cd vibing-steampunk && make build
```

Переменные окружения:
```bash
export SAP_URL=https://your-sap-host:44300
export SAP_USER=your-username
export SAP_PASSWORD=your-password
export SAP_CLIENT=001          # по умолчанию
export SAP_MODE=focused        # focused (48 инструментов) или expert (96)
```

Подробнее: [VSP README](https://github.com/oisee/vibing-steampunk) | [MCP Usage Guide](https://github.com/oisee/vibing-steampunk/blob/main/MCP_USAGE.md)
