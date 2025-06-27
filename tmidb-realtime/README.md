# tmiDB Realtime Extension

This project provides real-time data streaming capabilities via WebSockets for the tmiDB platform.

## Services Included

- **realtime**: A Go service that acts as a WebSocket hub.

## Functionality

- Allows clients to establish a WebSocket connection to receive real-time updates.
- Subscribes to relevant topics on the NATS message bus.
- Forwards messages from NATS to connected WebSocket clients, enabling real-time UIs and applications.
