# Go URL Shortener Engine ⚡

A high-throughput, low-latency URL shortener service built with Go. Features Base62 deterministic encoding, lock-free atomic analytics counters, interface-driven storage layer, and graceful HTTP server shutdown.

## Features

- Base62 deterministic hash encoding
- Non-blocking atomic counters for click analytics (`sync/atomic`)
- Thread-safe thread-synchronized state using `sync.RWMutex`
- Production-ready graceful HTTP shutdown handling
- Docker multi-stage containerization

## API Endpoints

### 1. Create Short URL
- **POST** `/shorten`
- **Body:** `{"url": "https://example.com"}`

### 2. Redirect
- **GET** `/{short_id}`

### 3. Get Click Metrics
- **GET** `/stats/{short_id}`

## Running Locally

```bash
go run main.go
