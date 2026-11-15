# PRD Assistant: LLM-Powered PRD Builder

**Go · Gin · React · MySQL · Docker · LLM API**

An AI agent that interviews you, one question at a time, and builds a Product Requirements Document (PRD) with you. Answer by typing or by voice. Watch the document fill in live. Edit any block with a GitHub-style before/after diff. Download the result as Word or PDF.

---

## 1. What it does

| Step | What happens |
|---|---|
| **LLM gate** | The first screen asks you to either enter an **API key** (OpenAI, Anthropic or Gemini) or connect a **local LLM** (Ollama or LM Studio). If neither works, the app tells you one is required and won't continue. This happens again every time the server restarts. |
| **Choose PRD type** | **Product**, **Project** or **Feature**. Each has its own template with its own sections and questions (Product and Feature follow the article's templates). |
| **Guided Q&A** | Questions come in PRD order. Sub-questions break big topics down (a persona asks for name, then designation, then pain points). You can answer several at once and the agent fills in the later questions. Optional questions can be skipped. |
| **Tags** | Every answer can carry tags: **P0 / P1 / P2** (priority), **V1 / V2 / V3 / FUTURE** (release), **IMPORTANT / NEEDS REVIEW / OPEN QUESTION** (status). Tags also filter what the generated PRD includes. |
| **Quality checks** | The LLM checks each answer against PRD quality rules: goals must be measurable, problems must name who is affected, requirements need acceptance criteria and a P0–P2 priority, and so on. Weak answers get feedback and a suggested rewrite. You can revise or choose "Keep as is". |
| **Live preview** | The right-hand pane updates after every answer. Click any block to edit it. |
| **Edit with diff + history** | Edits show a **before/after diff** and need your confirmation. You can also ask the AI to rewrite a block ("make this measurable"). The **last 3 versions** of each block are kept, with one-click restore. |
| **Polished PRD** | One click asks the LLM to turn your answers into a clean, structured document. |
| **Export** | Download as **Word (.docx)** or **PDF**. |
| **Memory** | Sessions, answers, chat and version history are stored in MySQL. Refresh the page or restart the server and nothing is lost, and you can resume any earlier PRD. The API key is the one exception: it is held in server memory only and never saved. |

Voice dictation uses the browser's built-in speech recognition, so it works in **Chrome, Edge and Safari** (not Firefox).

---

## 2. Getting an LLM (pick one)

> **Important:** A ChatGPT or Claude *chat subscription* (including the free plan) does **not** include an API key. API keys come from the developer consoles below.

| Option | Cost | Needs a download? | How |
|---|---|---|---|
| **Google Gemini API key** (recommended) | **Free tier**, no card | No | Sign in with a Google account at <https://aistudio.google.com/apikey> → **Create API key**. In the app pick "Google Gemini". |
| **Groq API key** | **Free tier**, no card | No | <https://console.groq.com/keys> → create a key. In the app pick "Groq". |
| **OpenRouter API key** | Free models available | No | <https://openrouter.ai/keys>. In the app pick "OpenRouter" (the app only uses models marked `:free`). |
| **OpenAI API key** | **Paid only**: a new account needs prepaid credit, otherwise you get a 429 error | No | <https://platform.openai.com/api-keys> |
| **Anthropic (Claude) API key** | **Paid only** (prepaid credit) | No | <https://console.anthropic.com/settings/keys> |
| **Local LLM with Ollama** | Free, offline, private | **Yes** (a model is about 2 GB+ and needs a decent laptop) | Install <https://ollama.com>, then `ollama pull llama3.2`. Choose "Use a local LLM". |

Leave the **Model** box empty: the app asks the provider which models your key can use and picks one that works, so retired model names are not a problem.

Free-tier limits and model names change over time. If a key is rejected or rate-limited, the app shows the reason.

---

## 3. How to run it

### Option A: Docker (recommended, one command)

**Needs:** [Docker Desktop](https://www.docker.com/products/docker-desktop/) only.

```bash
docker compose up --build
```

- App: **http://localhost:3000**
- API docs (Swagger): **http://localhost:8080/swagger**

The first build takes a few minutes. Stop with `Ctrl+C`; run `docker compose down` to remove the containers (your data stays in a Docker volume; add `-v` to wipe it).

*Using Ollama with Docker?*
- **Ollama installed on your computer:** nothing extra to do. The app reaches it at `host.docker.internal:11434`.
- **Ollama inside Docker:**
  ```bash
  OLLAMA_URL=http://ollama:11434 docker compose --profile ollama up --build
  docker compose exec ollama ollama pull llama3.2     # once
  ```

### Option B: Run locally without Docker

**Needs:** Go 1.25+, Node 20+. MySQL 8 is optional (see below).

```bash
# Terminal 1: backend  (STORE=memory = no database needed; data is lost on restart)
cd backend
STORE=memory go run ./cmd/server

# Terminal 2: frontend
cd frontend
npm install
npm run dev
```

Open **http://localhost:5173**.

To use MySQL instead of memory, create a database and user, then start the backend with a DSN:

```bash
mysql -uroot -e "CREATE DATABASE prd_assistant; CREATE USER 'prd'@'%' IDENTIFIED BY 'prdpass'; GRANT ALL ON prd_assistant.* TO 'prd'@'%';"
cd backend
MYSQL_DSN='prd:prdpass@tcp(localhost:3306)/prd_assistant?parseTime=true&charset=utf8mb4' go run ./cmd/server
```

Tables are created automatically on startup.

### Configuration (environment variables, backend)

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | API port |
| `STORE` | `mysql` | `mysql` or `memory` |
| `MYSQL_DSN` | `prd:prdpass@tcp(localhost:3306)/prd_assistant?...` | MySQL connection string |
| `OLLAMA_URL` | `http://localhost:11434` | Default address for "Use a local LLM" |

### Run the tests

```bash
cd backend && go test ./...
```

The end-to-end test drives the whole flow against a mock LLM: gate, multi-answer extraction, quality checks, edit/diff/history, tag filter, generate and Word/PDF export.

---

## 4. Suggested interview demo (about 3 minutes)

1. Open the app. It shows the **LLM gate**. Connect a key or local model.
2. Choose **Feature PRD** (the shortest template).
3. Answer the first question using the **🎤 mic**.
4. Give a deliberately vague goal ("make checkout better"). Show the **quality feedback and suggested rewrite**, then fix it.
5. Add a requirement and show that it asks for a **P0–P2 priority tag**.
6. Click a block in the **live preview**. Edit it and show the **before/after diff** and the **recent changes** list.
7. Click **Generate polished PRD**, then **download the PDF/Word**.
8. Open **/swagger** to show the documented REST API.

---

## 5. How it's built

```
PRD-Generator/
├── backend/                       Go + Gin
│   ├── cmd/server/main.go         entry point
│   └── internal/
│       ├── agent/                 the agent
│       │   ├── agent.go           main loop: ask → review → store → preview, edit flow
│       │   ├── planner.go         decides the next question (template order, sub-questions, repeats)
│       │   ├── memory.go          rebuilds LLM context from stored answers
│       │   └── preview.go         live document + tag filtering
│       ├── tools/                 what the agent can use
│       │   ├── registry.go        tool registration and discovery (GET /api/tools)
│       │   ├── validator.go       LLM quality check + P0–P2 rule
│       │   ├── diff.go            word-level before/after diff
│       │   └── exporter.go        .docx (hand-built OOXML) and .pdf (fpdf)
│       ├── llm/                   client.go (OpenAI-compatible, Anthropic, local) · prompt.go
│       ├── templates/defs/        product.yaml · project.yaml · feature.yaml  (questions live here)
│       ├── store/                 MySQL (+ in-memory for tests) and migrations
│       └── api/                   Gin handlers, openapi.yaml, Swagger UI
├── frontend/                      React + Vite
│   └── src/components/            LLMGate · TypePicker · Workspace (chat) · Preview · EditModal · TagPicker
└── docker-compose.yml             mysql + backend + frontend (+ optional ollama)
```

**Design choices worth mentioning**
- **Agent loop with tools.** A planner picks the next question from a YAML template, the validator tool reviews each answer, and the store is the agent's memory. The structure follows the Go agent layout (`core / tools / llm`) from the referenced article.
- **Templates are data.** Adding a question or a new PRD type means editing YAML, with no code changes.
- **One LLM interface, several backends.** OpenAI, Gemini, Ollama and LM Studio share the OpenAI-compatible wire format. Anthropic has its own client.
- **No context loss.** Every edit snapshots the previous value (last 3 kept per block), and every LLM call is rebuilt from the database.
- **Templates follow the referenced article.** **Product** and **Feature** use the article's two PRD templates, section for section. Product: Problem → Vision & Opportunity → Target Use Cases → Landscape → Proposed Solution (top 3 MVP value props) → Goals → Non-Goals → Y1 Success Metrics (goal / signals / metric / target) → Conceptual Model → Requirements (P0/P1/P2) → Appendix. Feature: Problem → Target Use Cases → Current User Journeys → Landscape → Proposed Solution → Goals → Out-of-scope → Measurable Outcomes → Requirements → Appendix. **Project** has no template in the article, so it uses a project-oriented structure (objectives and KPIs, stakeholders, deliverables, timeline, resources, risks).

---

## 6. Good to know

- **No login yet.** All sessions on a server are visible to whoever opens the app.
- **API key handling.** It lives only in the running server's memory and is never written to the database or logs. After a server restart you re-enter it (by design).
- **PDF fonts.** PDF export uses built-in fonts, so characters outside Western European languages (e.g. Hindi, emoji) won't render in the PDF. Word export handles them fine.
- **Quality checks need the LLM.** If the LLM call fails mid-session, the answer is still saved and the chat says the check was skipped.
- **Swagger UI** loads its viewer from a CDN, so `/swagger` needs internet. The raw spec is always at `/openapi.yaml`.
