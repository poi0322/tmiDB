# tmiDB MQTT Extension

This project provides MQTT ingestion capabilities for the tmiDB platform.

## Services Included

- **EMQX**: A high-performance MQTT broker.
- **ingestor**: A Go service that connects to the MQTT broker, subscribes to topics, and forwards messages to the NATS message bus for processing by the core `worker` service.

## Functionality

- Allows IoT devices and other clients to publish data to tmiDB over the MQTT protocol.
- Decouples data ingestion from data processing via the NATS message bus.
