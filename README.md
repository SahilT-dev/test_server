# WhatsApp Server for Render Deployment

This is a Go-based WhatsApp server using whatsmeow library that can be deployed on Render.

## Features
- WhatsApp Web API integration
- Message forwarding to ADK agent
- QR code generation for authentication
- Media file handling
- SQLite database for session storage

## Environment Variables Required

Set these in your Render dashboard:

```
DUMMY_AGENT_BASE_URL=https://your-agent-url.onrender.com
SERVER_BASE_URL=https://your-whatsapp-server.onrender.com
PORT=8080
```

## Deployment

### Option 1: Using Dockerfile (Recommended)
1. Connect repository to Render
2. Select "Docker" as environment
3. Render will automatically use the Dockerfile

### Option 2: Using Go Build
1. Connect repository to Render
2. Select "Go" as environment
3. Build Command: `go build -o main .`
4. Start Command: `./main`

## API Endpoints

- `GET /api/qr` - Get QR code for WhatsApp login
- `POST /api/send` - Send message to WhatsApp
- `GET /api/messages` - Get received messages
- `GET /api/download/{messageID}` - Download media files

## Local Development

```bash
# Install dependencies
go mod tidy

# Set environment variables
export DUMMY_AGENT_BASE_URL=http://localhost:8001
export SERVER_BASE_URL=http://localhost:8080

# Run the server
go run main.go
```

## Notes

- The server uses SQLite for local storage
- Media files are stored in memory (consider using external storage for production)
- QR code needs to be scanned within a few minutes of generation
- The server automatically forwards messages to the configured agent URL

## Architecture

```
WhatsApp Web ↔ This Server ↔ Your ADK Agent
```

The server acts as a bridge between WhatsApp Web and your ADK agent, handling authentication and message forwarding.
