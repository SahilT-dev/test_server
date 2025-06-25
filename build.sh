# Render Build Script for WhatsApp Server
#!/bin/bash

# Install dependencies
go mod tidy

# Build the application
go build -o main ./main.go

echo "Build completed successfully"
