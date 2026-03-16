# AGENTS.md

## Cursor Cloud specific instructions

### Project Structure

This is a monorepo with 3 main components:

| Component | Path | Language | Port |
|---|---|---|---|
| **AnyClaw-Server** | `AnyClaw-Server/` | Go 1.25.7 | 18790 (gateway) |
| **AnyClaw-API** | `AnyClaw-API/` | Go 1.22 | 8080 |
| **AnyClaw-Web** | `AnyClaw-Web/` | TypeScript (React+Vite) | 5173 (dev) |

`AnyClaw-Quicker/` is a Windows-only Quicker action (C#) and is not part of the main platform.

### System Dependencies

- **Go 1.25.7** required (Server needs 1.25.7, API needs 1.22; a single 1.25.7 install covers both). Installed at `/usr/local/go/bin/go`.
- **Node.js 20+** for AnyClaw-Web (npm as package manager, lockfile: `package-lock.json`).
- **MySQL 8.0** for AnyClaw-API.

### Running Services

**MySQL**: Start with `sudo mysqld --user=mysql --daemonize`. DB name: `anyclaw`, user: `anyclaw`, password: `anyclaw`. Socket at `/var/run/mysqld/mysqld.sock` — if permission denied, run `sudo chmod 755 /var/run/mysqld`.

**AnyClaw-API**:
```
ANYCLAW_CONFIG_PATH=/tmp/anyclaw-data/config.json go run ./cmd/api -config /tmp/anyclaw-data/config.json
```
Config file at `/tmp/anyclaw-data/config.json` with DB DSN, JWT secret, etc. If the config file doesn't exist, create it (see config setup below). The API auto-migrates the database schema on first connect.

**AnyClaw-Web** (dev server):
```
cd AnyClaw-Web && npx vite --host 0.0.0.0 --port 5173
```
Requires `VITE_API_URL=http://localhost:8080` in `.env` (copy from `.env.example`).

**AnyClaw-Server** (build only — requires LLM API key to actually run):
```
cd AnyClaw-Server && make build
```

### Key Gotchas

- **CORS**: A CORS middleware (`go-chi/cors`) was added to `AnyClaw-API/cmd/api/main.go` to allow the Vite dev server (port 5173) to communicate with the API (port 8080). In production the web SPA is embedded into the API binary so CORS is not needed.
- **ESLint config missing**: `AnyClaw-Web` has an `npm run lint` script but no `eslint.config.js` file. ESLint 9.x requires this file. Lint will fail with "couldn't find eslint.config" error. TypeScript checking (`tsc -b`) works fine.
- **AnyClaw-Server tests**: `TestShellTool_TimeoutKillsChildProcess` may fail in containerized environments due to process management limitations. This is a known environment-specific issue.
- **AnyClaw-API `go vet`**: Reports IPv6 address format warnings in `internal/mail/mail.go`. This is a pre-existing code issue.
- **AnyClaw-Server `go generate`**: Must run before build (the Makefile `build` target handles this via the `generate` dependency). It embeds the `workspace/` directory into the binary.

### Config Setup

If `/tmp/anyclaw-data/config.json` doesn't exist:
```json
{
  "port": 8080,
  "db_dsn": "anyclaw:anyclaw@tcp(localhost:3306)/anyclaw?parseTime=true&charset=utf8mb4",
  "jwt_secret": "dev-secret-key-for-testing",
  "api_url": "http://localhost:8080",
  "docker_image": "anyclaw/anyclaw"
}
```

### Commands Reference

| Task | Command |
|---|---|
| Build API | `cd AnyClaw-API && go build -o /tmp/anyclaw-api ./cmd/api` |
| Build Server | `cd AnyClaw-Server && make build` |
| Test Server | `cd AnyClaw-Server && make test` |
| Test API | `cd AnyClaw-API && go test ./...` |
| Vet Server | `cd AnyClaw-Server && make vet` |
| Vet API | `cd AnyClaw-API && go vet ./...` |
| Web build | `cd AnyClaw-Web && npm run build` |
| Web TS check | `cd AnyClaw-Web && npx tsc -b --noEmit` |
