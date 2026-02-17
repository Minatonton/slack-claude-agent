#!/bin/bash
set -e

INSTANCE="slack-claude-agent"
ZONE="asia-northeast1-b"

echo "Building..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server

echo "Uploading..."
gcloud compute scp bin/server ${INSTANCE}:/tmp/server --zone=$ZONE

echo "Installing and restarting..."
gcloud compute ssh $INSTANCE --zone=$ZONE --command="sudo mv /tmp/server /opt/slack-claude-agent/server && sudo chmod +x /opt/slack-claude-agent/server && sudo systemctl restart slack-claude-agent"

echo "Done!"
