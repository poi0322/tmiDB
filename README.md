# tmiDB: The Database That Connects All Information in Real-Time

**tmiDB (Too-Much-Information Database)** is a new-concept database born to collect, manage, and stream information about every **Target** in the world in real-time.

Built upon proven open-source technologies like PostgreSQL, TimescaleDB, and PostgREST, it offers both stability and scalability. Composed entirely of permissive licenses such as MIT and Apache-2.0, it can be used in any commercial service without restriction.

## Core Concepts

tmiDB operates on three core elements:

- **Target**: The central entity for information collection. It can be a person, an IoT device, a specific service, or even an abstract concept, uniquely identified by a `target_id`.

- **Speaker**: Any entity that generates and writes information about a 'Target' to tmiDB. This can range from direct data transmission via REST API, executing DB queries, to even an 'automated speaker' where tmiDB periodically fetches information from a registered URL.

- **Listener**: An entity that subscribes to a 'Target's' information and receives real-time updates. Once registered as a 'Listener' for a specific target, an API endpoint is automatically generated to access its data, and all changes can be streamed in real-time via WebSocket.

## Key Features

- **Flexible Dynamic Schema**: Like MongoDB, there's no need to pre-define attributes (i.e., columns) for a target. You can freely add or modify any type of information through the `meta` field, and its structure is versioned internally whenever it changes.

- **Automated API Generation & Versioning**: When the data schema changes, the REST API is instantly and automatically updated and versioned via PostgREST. This ensures that the data model and API always remain consistent, enabling stable service operation similar to gRPC.

- **Powerful Time-Series and Geospatial Data Processing**: It efficiently handles large-scale time-series data using TimescaleDB as a hypertable and easily performs complex location-based queries (radius search, proximity analysis, etc.) with PostGIS's `cube` and `earthdistance` extensions.

- **Real-Time Streaming**: Through the Supabase Realtime engine, all database changes (INSERT, UPDATE, DELETE) are instantly delivered to Listeners.

## Getting Started

```bash
# Run all services at once with Docker Compose.
docker-compose up -d

# Create a new 'Target'.
curl -X POST http://localhost:3000/target \
  -H "Content-Type: application/json" \
  -d '{ "name": "server-01", "meta": { "ip": "192.168.0.10", "status": "online" } }'

# Add CPU usage data for 'server-01' (acting as a Speaker).
curl -X POST http://localhost:3000/ts_obs \
    -H "Content-Type: application/json" \
    -d '{ "target_id": "<target_id_of_server-01>", "payload": { "cpu_usage": 15.5 } }'

# Subscribe to all changes for 'server-01' in real-time with a WebSocket client (acting as a Listener).
# (Refer to the Realtime section in the documentation for detailed instructions.)
```

tmiDB will become the most powerful tool for clearly connecting intricately intertwined data and creating a living flow of information.
