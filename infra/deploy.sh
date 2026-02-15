#!/bin/bash
set -e

INSTANCE="slack-claude-agent"
ZONE="asia-northeast1-b"

echo "Building..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/server ./cmd/server

echo "Uploading..."
gcloud compute scp bin/server ${INSTANCE}:/opt/slack-claude-agent/server --zone=$ZONE

echo "Restarting..."
gcloud compute ssh $INSTANCE --zone=$ZONE --command="sudo systemctl restart slack-claude-agent"

echo "Done!"
