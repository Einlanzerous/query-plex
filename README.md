# plex-mcp-go

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server written in Go that connects AI assistants to a remote Plex Media Server. It allows an LLM to search your Plex library and check what's currently on deck — all through natural language.

Built with [mcp-go](https://github.com/mark3labs/mcp-go).

## What It Does

This server exposes two MCP tools over stdio:

### `search_media`

Searches the Plex library using fuzzy matching (via the `/hubs/search` endpoint).

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Search term (e.g. "batman", "breaking bad") |
| `media_type` | string | no | Filter results: `"movie"` or `"show"` |

Returns a numbered list of matching titles with year and media type.

### `get_on_deck`

Fetches the "On Deck" / "Continue Watching" list for the authenticated user. Takes no parameters.

Returns each item with:
- Show name and episode identifier (e.g. S02E05) for TV
- Watch progress percentage for partially-watched items

## Prerequisites

- Access to a Plex Media Server (local or remote)
- A valid [Plex authentication token](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/)
- **Docker** (recommended) or **Go 1.23+** for local builds

## Setup

### 1. Get Your Plex Token

Follow the [official Plex guide](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/) to find your `X-Plex-Token`. The quickest method:

1. Open Plex Web and browse to any media item.
2. Click "Get Info" > "View XML".
3. The URL will contain `X-Plex-Token=<your-token>`.

### 2. Create Your `.env` File

```bash
cp .env.example .env
```

Edit `.env` with your values:

```
PLEX_URL=https://your-plex-server.example.com:32400
PLEX_TOKEN=your-plex-token-here
```

### 3. Run

#### Docker (recommended)

```bash
docker compose up --build
```

The resulting image uses a multi-stage build (`golang:1.23-alpine` to compile, `distroless/static` for runtime) and is under 20MB.

#### Local

```bash
export PLEX_URL=https://your-plex-server.example.com:32400
export PLEX_TOKEN=your-plex-token-here
go run .
```

## Connecting to an MCP Client

This server communicates over **stdio**, the standard transport for MCP. To use it with an MCP-compatible client, point the client at the binary or Docker container.

### Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, or `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "plex": {
      "command": "/path/to/plex-mcp-go",
      "env": {
        "PLEX_URL": "https://your-plex-server.example.com:32400",
        "PLEX_TOKEN": "your-plex-token-here"
      }
    }
  }
}
```

### Claude Code

Add to your Claude Code MCP settings (`.claude/settings.json` or project-level):

```json
{
  "mcpServers": {
    "plex": {
      "command": "/path/to/plex-mcp-go",
      "env": {
        "PLEX_URL": "https://your-plex-server.example.com:32400",
        "PLEX_TOKEN": "your-plex-token-here"
      }
    }
  }
}
```

To build a standalone binary:

```bash
go build -o plex-mcp-go .
```

## Startup Validation

On launch, the server pings the Plex API (`/identity` endpoint) to verify your token is valid. If the connection fails or the token is rejected, the server exits with a fatal error immediately rather than starting in a broken state.

## Project Structure

```
.
├── main.go              # MCP server, tool handlers, Plex API client
├── go.mod               # Go module definition
├── go.sum               # Dependency checksums
├── Dockerfile           # Multi-stage build (builder → distroless)
├── docker-compose.yml   # Docker Compose service definition
├── .env.example         # Template for environment variables
└── README.md
```
