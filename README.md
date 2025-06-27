# tmiDB Project

A multi-component database platform for time-series, geospatial, and unstructured data.

## Structure

This project is a monorepo containing several independent services:

- `tmidb-core/`: The core tmiDB service, including the database, API, and web console.
- `tmidb-mqtt/`: The MQTT extension for data ingestion.
- `tmidb-realtime/`: The WebSocket extension for real-time features.

Each project has its own `README.md` with detailed information.

## How to Run

### Prerequisites

1. Ensure Docker is installed.

### Development Mode (with Live Reloading)

This command starts all services in development mode. Changes to the source code in any of the `tmidb-*` directories will trigger an automatic restart of the corresponding service.

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

### Production Mode

This command starts all services in production mode, using the pre-built binaries within the images.

```bash
docker compose up --build
```

### Stopping Services

```bash
docker compose down
```
