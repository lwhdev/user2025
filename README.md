# Go MySQL Auth API

This project is a minimal Go web API that exposes user registration, login and profile retrieval backed by a MySQL database. Authentication tokens are implemented with JWT and the application ships with Docker assets for local development.

## Features

- User registration with salted SHA-256 password hashing.
- Login endpoint issuing short-lived JWT tokens.
- Protected profile endpoint requiring a valid bearer token.
- Automatic database and table bootstrap during startup using the bundled MySQL CLI wrapper.
- Dockerfile and `docker-compose.yml` for running the API alongside MySQL.

## Configuration

Environment variables control the database connection and runtime behaviour:

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | HTTP port for the API. |
| `DB_HOST` | `127.0.0.1` | MySQL host. |
| `DB_PORT` | `3306` | MySQL port. |
| `DB_USER` | `root` | MySQL user. |
| `DB_PASSWORD` | `password` | MySQL password. |
| `DB_NAME` | `app_db` | Database name. |
| `JWT_SECRET` | `development-secret` | Secret used to sign JWT tokens. |
| `MYSQL_CLIENT_PATH` | _(unset)_ | Optional absolute path to the `mysql` client binary. |

## Running locally

1. Ensure you have Docker installed.
2. Start the stack:

   ```bash
   docker-compose up --build
   ```

3. Interact with the API:

   ```bash
   # Register a new user
   curl -X POST http://localhost:8080/api/v1/register \
     -H "Content-Type: application/json" \
     -d '{"email":"user@example.com","password":"password123","full_name":"Demo User"}'

   # Login and capture the token
   curl -X POST http://localhost:8080/api/v1/login \
     -H "Content-Type: application/json" \
     -d '{"email":"user@example.com","password":"password123"}'

   # Use the token to access the profile endpoint
   curl http://localhost:8080/api/v1/profile \
     -H "Authorization: Bearer <token>"
   ```

## Development

Run the unit tests (no MySQL required):

```bash
go test ./...
```

The API relies on the `mysql` command-line client under the hood, so ensure it is installed and available on your `$PATH` when running outside Docker (the provided Docker image already includes it).
