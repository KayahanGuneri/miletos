# Miletos

## Host development

Start the Go runtime normally from `miletos-be/miletos-go`:

```powershell
go run ./cmd/engine
```

## Full Docker stack

Apply the database migrations required by the project's release or migration notes before starting the application stack. Local SQL files under `miletos-be/miletos-go/.local/migrations` are Git ignored and are not copied into images or executed automatically by Docker.

Create the runtime environment file once, replace the placeholder shared token and AES key for any non-local deployment, then start the schema-prepared stack from the repository root:

```powershell
Copy-Item miletos-be/miletos-go/.env.example miletos-be/miletos-go/.env
docker compose up -d --build
```

The default host endpoints are frontend `http://localhost:3000`, backend `http://localhost:8080`, runtime HTTP `http://localhost:8081`, runtime gRPC `localhost:9091`, MailHog `http://localhost:8025`, PostgreSQL `localhost:55432`, output PostgreSQL `localhost:55480`, Redis `localhost:6379`, Kafka `localhost:9092`, and SFTP `localhost:2222`.

Browser-facing API and trigger URLs intentionally use `localhost`; container-to-container traffic uses Compose service names. SFTP host and port are workflow configuration. Use `127.0.0.1:2222` when the Go runtime runs on the host and `sftp:22` for workflows created for the Docker runtime. Existing persisted workflows are not rewritten automatically.

External PostgreSQL connections are also workflow-node configuration, not runtime-wide environment settings. Both `core.database-input` and `core.database-output` use `databaseHost`, `databasePort`, `databaseName`, `databaseUsername`, `databasePassword`, and `databaseSslMode`; the Java control plane persists the password only as `databasePasswordEncrypted`. For the Compose sample database, use `output-postgres:5432` from the Docker runtime or `127.0.0.1:55480` from a host-run runtime. Existing database nodes must be edited to add these fields; definitions are not migrated or given an application-level fallback.

The Java backend and Go runtime share the `miletos-workflow-inputs` volume for uploaded local workflow inputs. Profile photos, runtime outputs, both PostgreSQL databases, Kafka data, and SFTP data use named volumes.
