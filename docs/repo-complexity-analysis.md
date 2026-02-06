# Repo Complexity Analysis: `slack-mcp-server`

This analysis was produced to calibrate a planning orchestrator that decides which model tier (haiku/sonnet/opus) to use for explore agents based on repo complexity.

## 1. Source-Only Sizing

| Metric | Value |
|--------|-------|
| Source files | **35** (34 `.go` + 1 `.js`) |
| Total lines | **10,284** |
| Mean lines/file | **293.8** |
| Median lines/file | **162** |
| Language | Go (99.4% of source lines) |

This is a **small repo** by any measure. 10K lines is roughly what a single senior engineer writes in 2-3 focused weeks.

## 2. File Size Distribution

| Bucket | Files | % Files | Lines | % Lines |
|--------|------:|--------:|------:|--------:|
| 0-50 | 4 | 11.4% | 80 | 0.8% |
| 51-100 | 5 | 14.3% | 295 | 2.9% |
| 101-200 | 12 | 34.3% | 1,729 | 16.8% |
| 201-500 | 8 | 22.9% | 2,704 | 26.3% |
| 501-1000 | 4 | 11.4% | 2,864 | 27.8% |
| 1000+ | 2 | 5.7% | 2,612 | 25.4% |

The distribution has a heavy tail. 6 files (17.1%) hold **53.2%** of all code. The modal bucket is 101-200 lines (12 files), which are mostly lean, single-responsibility modules.

## 3. Non-Source File Inventory

| Category | Count | Total Lines/Size | Architecturally Relevant? |
|----------|------:|----------------:|---------------------------|
| Go module files (`go.mod`, `go.sum`) | 2 | 366 lines | **Yes** — `go.mod` reveals all dependencies; `go.sum` is noise |
| Markdown docs (README, QUICKSTART, etc.) | 8 | ~855 lines | **Partially** — `CLAUDE.md` and `docs/03-configuration-and-usage.md` document architecture; rest is user-facing |
| Makefile | 1 | 124 lines | **Yes** — defines build targets, cross-compilation, release process |
| Dockerfile + docker-compose (3) | 4 | ~121 lines | **Minor** — deployment concerns, not core logic |
| GitHub Actions workflows | 4 | 160 lines | **No** — CI config, standard patterns |
| JSON configs (`manifest-dxt.json`, package.json x7) | 8 | ~209 lines | **Minor** — npm distribution scaffolding + DXT manifest |
| Images (PNG, GIF) | 5 | ~6 MB binary | **No** — demo screenshots and icon |
| Test images (PNG/JPG) | 6 | ~5 MB binary | **No** — compression test fixtures |
| Config/IDE files (`.env.dist`, `launch.json`, `.gitignore`) | 5 | ~73 lines | **No** — environment setup |

**Safe to filter**: Images, test fixtures, npm platform packages, CI workflows, docker configs, gitignore files. These add zero to architectural understanding.

**Must NOT filter**: `go.mod` (dependency graph), `Makefile` (build system), `CLAUDE.md` (architectural overview), `manifest-dxt.json` (if understanding the DXT packaging model).

## 4. Top 10 Largest Source Files

| # | File | Lines | Imports | Funcs/Methods | Fan-In | Verdict |
|--:|------|------:|--------:|--------------:|-------:|---------|
| 1 | `pkg/handler/conversations.go` | 1,496 | 17 | ~31 | 0 (leaf) | **Legitimate hub** — core MCP tool implementations; large because it handles 4 distinct tools (history, replies, search, add_message) with param parsing, pagination, image extraction, and CSV formatting |
| 2 | `pkg/handler/images_test.go` | 1,116 | 9 | ~12 | 0 | **Test file** — comprehensive table-driven tests with large inline HTML fixtures; size is from test data, not logic |
| 3 | `pkg/provider/api.go` | 872 | 16 | ~37 | 6 | **Architectural spine** — the ApiProvider facade combining official Slack API + Edge API + caching layer; highest fan-in in the repo |
| 4 | `pkg/handler/conversations_test.go` | 786 | 17 | ~7 | 0 | **Integration test** — LLM-based integration tests using OpenAI; size from test setup |
| 5 | `pkg/provider/edge/client_boot.go` | 632 | 6 | 4 | 0 | **Data model file** — 41 type declarations modeling the Slack `client.userBoot` API response; almost no logic, mostly struct definitions |
| 6 | `pkg/handler/images.go` | 574 | 13 | ~14 | 0 | **Feature module** — image extraction from Slack messages with compression, SSRF protection, base64 encoding |
| 7 | `pkg/transport/transport.go` | 470 | 18 | ~10 | 1 | **Infrastructure** — HTTP client with uTLS fingerprinting, cookie management, proxy support; isolated and self-contained |
| 8 | `pkg/provider/edge/edge.go` | 403 | 17 | ~27 | 1 | **Edge API core** — HTTP request construction, authentication, JSON parsing for undocumented Slack APIs |
| 9 | `pkg/server/server.go` | 361 | 12 | ~5 | 1 | **Wiring hub** — registers all MCP tools, resources, and prompts; connects handlers to the MCP framework |
| 10 | `cmd/slack-mcp-server/main.go` | 348 | 13 | 8 | 0 | **Entry point** — CLI flag parsing, transport selection, cache warming, graceful shutdown |

No God objects. No bloated monoliths. The largest file (`conversations.go`) is large because it implements 4 related MCP tools — a reasonable grouping.

## 5. Hub / High-Coupling Files (>15 imports)

| # | File | Import Count | Role |
|--:|------|------------:|------|
| 1 | `pkg/transport/transport.go` | 18 | HTTP/TLS client construction — pulls in crypto, networking, uTLS |
| 2 | `pkg/provider/edge/edge.go` | 17 | Edge API client — HTTP, JSON, auth, tracing |
| 3 | `pkg/handler/conversations.go` | 17 | Core MCP tools — Slack SDK, MCP framework, CSV, text processing |
| 4 | `pkg/handler/conversations_test.go` | 17 | Integration test — test frameworks, OpenAI SDK, HTTP server |
| 5 | `pkg/provider/api.go` | 16 | API facade — combines all provider dependencies |

Only **5 files** exceed 15 imports, and 1 is a test file. The remaining 4 are exactly the files you'd expect to be coupling points: the transport layer, the API facade, the edge client core, and the main handler.

### Internal Package Fan-In

| Package | Fan-In | Role |
|---------|-------:|------|
| `pkg/provider` | 6 | Most depended-upon — the Slack abstraction layer |
| `pkg/limiter` | 5 | Rate limiting used across edge sub-packages |
| `pkg/text` | 4 | Text processing utilities |
| `pkg/server/auth` | 3 | Auth middleware |
| `pkg/provider/edge/fasttime` | 2 | Internal to edge package |
| `pkg/test/util` | 2 | Test helpers |
| All others | 1 each | Properly isolated |

## 6. Complexity Concentration

| Metric | Value |
|--------|-------|
| Top 5% of files (1 file) | **1,496 lines = 14.5%** of total |
| Top 10% of files (3 files) | **3,484 lines = 33.9%** of total |
| Top 20% of files (7 files) | **5,916 lines = 57.5%** of total |
| Boilerplate files (<10 lines) | **1** (`pkg/version/version.go` — 6 lines) |
| Small files (<50 lines) | **4** (80 lines total = 0.8%) |

Concentration is moderate. The top 10% holds a third of code, but this is mostly `conversations.go` (the main handler) and test files. It's not pathological — there's no single file that would collapse the project if misunderstood.

## 7. Assessment

### Effective Size

- **35 source files, 10,284 lines total**
- **~25 files contain meaningful logic** (excluding test files, boilerplate, and platform stubs)
- **~7,000 lines of production code** (excluding ~3,200 lines of tests)
- This is a **small, well-structured repo**. An experienced human could read every line in half a day.

### Where the Real Complexity Lives

The **5 architecturally critical files** an explore agent must find:

1. **`pkg/provider/api.go`** (872 lines) — The architectural spine. Dual API client pattern (official + edge), cache-first startup, channel/user resolution. If you misunderstand this file, you misunderstand the repo.
2. **`pkg/handler/conversations.go`** (1,496 lines) — All 4 core MCP tools. Message formatting, pagination logic, image extraction, search filtering.
3. **`pkg/server/server.go`** (361 lines) — The wiring diagram. Shows how tools, resources, and prompts connect to the MCP framework.
4. **`pkg/provider/edge/edge.go`** (403 lines) — The undocumented API client. Understanding when/why the edge API is used vs the official API is a key architectural decision.
5. **`cmd/slack-mcp-server/main.go`** (348 lines) — Entry point. Transport mode selection, authentication routing, cache warming sequence.

Secondary files that flesh out the picture:

6. **`pkg/transport/transport.go`** (470 lines) — TLS fingerprinting for enterprise environments
7. **`pkg/handler/images.go`** (574 lines) — Image handling pipeline (extraction, compression, SSRF protection)
8. **`pkg/provider/edge/client_boot.go`** (632 lines) — Data model for Slack's undocumented boot API (understanding the type landscape)

### Model Recommendation: Haiku is adequate

Reasoning:

- **10K lines, 35 files** — well within haiku's context and reasoning capacity
- **Clean package structure** — standard Go idioms, no metaprogramming, no code generation, no framework magic
- **Low coupling** — max fan-in is 6, max imports is 18; no tangled dependency graphs
- **No accidental complexity traps** — the largest file (`conversations.go`) is long but linear; `client_boot.go` (632 lines) is just struct definitions
- **Clear naming** — packages are named what they do (`handler`, `provider`, `transport`, `text`)

### What Would Trip Up a Weaker Model

1. **The dual API pattern**: `api.go` seamlessly switches between `slack-go/slack` (official) and `edge.Client` (undocumented). A weak model might not notice these are two completely different API surfaces with different auth requirements, or conflate `github.com/rusq/slack` with `github.com/slack-go/slack` (they're different forks).

2. **The `client_boot.go` trap**: 632 lines of struct definitions with zero logic. A model that equates lines-of-code with complexity would waste time "analyzing" 41 data types that are just API response shapes. A good model skims it and moves on.

3. **The auth mode branching**: Three token types (`xoxp`, `xoxb`, `xoxc+xoxd`) create implicit behavioral branches throughout the provider. A weak model might miss that bot tokens (`xoxb`) can't search, or that browser tokens (`xoxc`) enable the edge API path.

4. **The cache-warming dependency**: The server **blocks startup** until user/channel caches are populated. This isn't obvious from function signatures alone — you need to trace the init sequence in `main.go` -> `api.go` -> `RefreshUsers()`/`RefreshChannels()`.

None of these require opus-level reasoning. They require **careful reading** more than **deep inference**. Haiku with good explore prompts (directing it to `api.go` and `main.go` first) would handle this fine. Sonnet would be overkill; opus would be wasted.

### Calibration Guideline

For repos under ~15K source lines with clear package structure and <40 files, haiku is the right choice. Save sonnet for repos with complex inheritance hierarchies, heavy metaprogramming, or >50K lines where architectural patterns are buried in noise.
