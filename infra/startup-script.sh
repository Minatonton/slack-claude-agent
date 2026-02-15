#!/bin/bash
set -e

# Install dependencies
apt-get update
apt-get install -y nginx certbot python3-certbot-nginx

# Create application user
useradd -r -s /bin/false slackbot || true

# Create application directory
mkdir -p /opt/slack-claude-agent
chown slackbot:slackbot /opt/slack-claude-agent

# Copy systemd service
cp /opt/slack-claude-agent/slack-claude-agent.service /etc/systemd/system/ 2>/dev/null || true
systemctl daemon-reload
systemctl enable slack-claude-agent

echo "Startup script complete. Deploy the application binary and .env file to /opt/slack-claude-agent/"
echo "Then run: sudo systemctl start slack-claude-agent"
echo "For HTTPS, run: sudo certbot --nginx -d bot.yourdomain.com"
