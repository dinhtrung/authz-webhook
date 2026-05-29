# Authorization Webhook for Ory Oathkeeper

This is a simple authorization webhook service written in Go. It integrates with Ory Kratos to check if an authenticated user has admin privileges, typically for use with Ory Oathkeeper's `remote_json` authenticator.

## How It Works

1.  Receives a request from Oathkeeper containing an `identity_id`.
2.  Checks an internal in-memory cache (using BadgerDB) for the user's admin status.
3.  If not cached, queries the Kratos Admin API to fetch the user's `metadata_admin.IsAdmin` property.
4.  Caches the result temporarily for performance.
5.  Responds to Oathkeeper with a `200 OK` (allowed) or `403 Forbidden` (denied).

## Configuration

The service can be configured via command-line flags or environment variables (or both). Command-line flags take precedence.

### Environment Variables

- `LISTEN`: Address to bind to (default: `:5000`).
- `KRATOS_ADMIN_URL`: Base URL for the Kratos Admin API (default: `http://127.0.0.1:4434/admin/identities`).

### Command-Line Flags

- `-listen`: Address to bind to (default from `LISTEN` or `:5000`).
- `-kratos-url`: Base URL for the Kratos Admin API (default from `KRATOS_ADMIN_URL` or `http://127.0.0.1:4434/admin/identities`).

Example:
```bash
./authz-webhook -listen :8080 -kratos-url http://my-kratos:4434/admin/identities
```

Or with environment variables:
```bash
LISTEN=:8080 KRATOS_ADMIN_URL=http://my-kratos:4434/admin/identities ./authz-webhook
```

## Docker

Build and run using the provided `Dockerfile` and `docker-compose.yml`.

## Development

Requires Go 1.25 or higher.

```bash
go mod tidy
go run main.go
```
