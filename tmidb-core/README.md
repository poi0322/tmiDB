# tmiDB Core

This project contains the core functionalities of the tmiDB platform.

## Services Included

- **Database (PostgreSQL + TimescaleDB)**: The main data store.
- **NATS**: The message bus for asynchronous communication.
- **SeaweedFS**: The object storage for large files (S3 compatible).
- **migrator**: A Go service that runs once to set up or migrate the database schema.
- **api**: The main Go service providing the REST API and web console.
- **worker**: A Go service for background jobs, processing messages from NATS.

## Functionality

- Provides the main database schema and handles migrations.
- Exposes a REST API for data interaction.
- Serves the web console for administration and data exploration.
- Processes background tasks and data writing asynchronously.
