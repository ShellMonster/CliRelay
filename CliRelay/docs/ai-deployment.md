# AI Deployment Guide

This document is for AI agents or users who ask an AI to deploy this repository automatically.

Read this before running any deployment command.

## Hard Rules

1. Do not assume `docker compose up -d` is enough.
2. Do not start the container before preparing `data/config.yaml`.
3. Do not treat `http://localhost:8317/v1` as a health-check URL.
4. Do not use the frontend URL as the backend API URL unless they are actually reverse-proxied together.
5. If the user wants the management panel, `remote-management.secret-key` must be set.
6. If the user wants to access the management panel from another machine, `remote-management.allow-remote: true` must also be set.

## Correct Backend Paths

- Server root: `http://localhost:8317/`
- OpenAI-compatible API base: `http://localhost:8317/v1`
- Management API base: `http://localhost:8317/v0/management`

Important:
- `GET /v1` is not a generic root endpoint and may return 404.
- Use `/` to verify the server is alive.

## Required Files Before Docker Startup

The Docker Compose service starts the backend with:

```bash
./CLIProxyAPI -config /data/config.yaml
```

So this file must exist before first startup:

```bash
data/config.yaml
```

Prepare it like this:

```bash
mkdir -p data
cp config.example.yaml data/config.yaml
```

## Minimum Management Configuration

If the user wants the frontend management panel, set at least:

```yaml
remote-management:
  allow-remote: false
  secret-key: "your-management-key"
```

Notes:
- If `secret-key` is empty, all `/v0/management/*` endpoints return `404`.
- If access is remote and `allow-remote` is `false`, management requests return `403`.

## Recommended Docker Commands

Use local source build explicitly:

```bash
docker compose up -d --build
```

Do not rely on a plain `docker compose up -d` when the goal is to run the checked-out repository code.

## Frontend Login Rules

If using the `codeProxy` frontend:

- Backend base URL should be `http://localhost:8317`
- Do not enter `http://localhost:8317/v1`
- Do not enter `http://localhost:8317/v0/management`

The frontend appends the management prefix automatically.

## Reverse Proxy Requirements

If frontend and backend are deployed behind a reverse proxy, the proxy must forward:

- `/v0/management/*` to CliRelay backend
- `/v1/*` to CliRelay backend

If these routes are not forwarded, the frontend may show 404 even when the backend is running.

## Quick Verification Checklist

After deployment:

1. Check server root:

```bash
curl http://localhost:8317/
```

2. If management is enabled, check config endpoint with management key:

```bash
curl -H "Authorization: Bearer your-management-key" \
  http://localhost:8317/v0/management/config
```

3. If using the frontend, log in with:

- API Base: `http://localhost:8317`
- Management Key: the value configured in `remote-management.secret-key`

## Typical Failure Causes

### Frontend login returns 404

Usually one of these:
- `data/config.yaml` was not prepared
- `remote-management.secret-key` is empty
- The frontend was pointed at the wrong backend base URL
- Reverse proxy did not forward `/v0/management/*`

### Frontend login returns 403

Usually:
- `remote-management.allow-remote` is `false`
- The request is coming from a non-localhost machine

### `http://localhost:8317/v1` does not open

Usually expected.

Use:
- `http://localhost:8317/` for liveness
- `http://localhost:8317/v1/...` for actual API calls

## Recommended AI Deployment Procedure

If an AI agent is deploying this repository, it should follow this order:

1. Create `data/config.yaml` from `config.example.yaml`
2. Ask for or set a valid `remote-management.secret-key` if management UI is needed
3. Add required API keys or auth configuration
4. Run `docker compose up -d --build`
5. Verify `/`
6. Verify `/v0/management/config` only if management is enabled
7. Use `http://localhost:8317` as the frontend login base
