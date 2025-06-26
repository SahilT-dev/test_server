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

### Core WhatsApp API
- `GET /api/qr` - Get QR code for WhatsApp login
- `POST /api/send` - Send message to WhatsApp
- `GET /api/messages` - Get received messages
- `GET /api/download/{messageID}` - Download media files

### Health & Monitoring
- `GET /health` - Health check endpoint with connection status
- `GET /status` - Server status with uptime and configuration
- `GET /` - Root endpoint (same as `/status`)
- `GET /api/health` - Alternative health check endpoint
- `GET /api/status` - Alternative status endpoint

#### Health Check Response Example
```json
{
  "status": "healthy",
  "timestamp": "2025-06-25T20:30:15Z",
  "server": "WhatsApp Server",
  "version": "1.0.0",
  "whatsapp_connected": true,
  "device_jid": "918384884150:9@s.whatsapp.net",
  "database_connected": true
}
```

#### Status Response Example
```json
{
  "service": "WhatsApp ADK Server",
  "status": "running",
  "uptime_seconds": 3600.5,
  "uptime_human": "1h0m0.5s",
  "timestamp": "2025-06-25T20:30:15Z",
  "server_url": "https://test-server-481n.onrender.com",
  "agent_url": "https://xxx.ngrok.io"
}
```

### Keep-Alive for Render Free Tier

To prevent the server from sleeping on Render's free tier, set up monitoring:

#### Option 1: UptimeRobot (Recommended)
1. Sign up at [UptimeRobot](https://uptimerobot.com/)
2. Add monitor: `https://your-server.onrender.com/health`
3. Set interval to 5 minutes

#### Option 2: GitHub Actions
Create `.github/workflows/keep-alive.yml`:
```yaml
name: Keep Server Alive
on:
  schedule:
    - cron: '*/10 * * * *'  # Every 10 minutes
jobs:
  ping:
    runs-on: ubuntu-latest
    steps:
      - name: Ping Server
        run: |
          curl -f https://your-server.onrender.com/health
          curl -f https://your-server.onrender.com/status
```

#### Option 3: Simple Cron Job
```bash
# Add to your crontab
*/10 * * * * curl -s https://your-server.onrender.com/health > /dev/null
```

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
