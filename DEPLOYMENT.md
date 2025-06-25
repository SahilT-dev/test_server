# WhatsApp Server Deployment on Render

## Prerequisites
1. GitHub account
2. Render account (free tier available)
3. Your WhatsApp ADK Agent already deployed or running

## Step-by-Step Deployment

### Step 1: Prepare Repository
1. Ensure all modified files are committed to your GitHub repository
2. The `whatsapp-server-implementation/whatsapp-server` folder should contain:
   - `main.go` (modified for dynamic URLs)
   - `Dockerfile`
   - `build.sh`
   - `render.yaml`
   - `go.mod` and `go.sum`

### Step 2: Deploy to Render
1. Go to https://render.com and sign in
2. Click "New +" and select "Web Service"
3. Connect your GitHub repository: `https://github.com/SahilT-dev/whatsapp_meow`
4. Configure the service:
   - **Name**: `whatsapp-server`
   - **Root Directory**: `whatsapp-server-implementation/whatsapp-server`
   - **Environment**: `Docker`
   - **Plan**: Free (sufficient for testing)

### Step 3: Environment Variables
Set these environment variables in Render dashboard:
```
DUMMY_AGENT_BASE_URL=https://your-agent-url.onrender.com
SERVER_BASE_URL=https://your-whatsapp-server.onrender.com
PORT=8080
```

### Step 4: Deploy
1. Click "Create Web Service"
2. Render will automatically build and deploy
3. Wait for deployment to complete (usually 5-10 minutes)

### Step 5: Update Agent Configuration
Once WhatsApp server is deployed, update your agent's `.env`:
```
GO_SERVER_URL=https://your-whatsapp-server.onrender.com
```

## Expected URLs
- **WhatsApp Server**: `https://your-whatsapp-server.onrender.com`
- **QR Code API**: `https://your-whatsapp-server.onrender.com/api/qr`
- **Message API**: `https://your-whatsapp-server.onrender.com/api/messages`
- **Send API**: `https://your-whatsapp-server.onrender.com/api/send`

## Testing
1. Visit `https://your-whatsapp-server.onrender.com/api/qr` to get QR code
2. Scan with WhatsApp to connect
3. Send messages to test the integration

## Troubleshooting
- Check Render logs for any build/runtime errors
- Ensure environment variables are set correctly
- Verify agent URL is accessible from the internet
