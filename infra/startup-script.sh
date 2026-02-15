#!/bin/bash
set -e

# Update system
apt-get update

# Install Node.js (required for Claude Code CLI)
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

# Install Claude Code CLI globally
npm install -g @anthropic-ai/claude-code

# Install git (required for Claude Code to create PRs)
apt-get install -y git

# Create application user with a shell (needed for Claude Code)
useradd -r -s /bin/bash slackbot || true
usermod -s /bin/bash slackbot || true

# Create application directory
mkdir -p /opt/slack-claude-agent
chown slackbot:slackbot /opt/slack-claude-agent

# Create home directory for slackbot user (Claude Code needs it)
mkdir -p /home/slackbot
chown slackbot:slackbot /home/slackbot

# Copy systemd service
cp /opt/slack-claude-agent/infra/slack-claude-agent.service /etc/systemd/system/ 2>/dev/null || true
systemctl daemon-reload
systemctl enable slack-claude-agent

echo "Startup script complete."
echo "Next steps:"
echo "1. Deploy the application binary to /opt/slack-claude-agent/server"
echo "2. Create /opt/slack-claude-agent/.env with required variables"
echo "3. Configure git for slackbot user: sudo -u slackbot git config --global user.name 'Bot Name'"
echo "4. sudo systemctl start slack-claude-agent"
