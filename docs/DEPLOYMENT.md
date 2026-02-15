# GCE デプロイガイド（Console 版）

## 事前準備

### 1. 必要な情報を集める

以下の情報をメモ帳などに準備してください：

```
# Slack
SLACK_BOT_TOKEN=xoxb-...          # Bot User OAuth Token
SLACK_APP_TOKEN=xapp-...          # App-Level Token

# GitHub
GITHUB_OWNER=your-org             # GitHub Organization/User
GITHUB_REPO=your-repo             # リポジトリ名
GITHUB_PAT=ghp_...                # Personal Access Token

# Commit Author
AUTHOR_NAME=Your Name
AUTHOR_EMAIL=you@example.com
```

### 2. Slack App の設定

#### App-Level Token 生成

1. https://api.slack.com/apps を開く
2. あなたの App を選択
3. **Settings** → **Basic Information** を開く
4. **App-Level Tokens** セクションで **Generate Token and Scopes** をクリック
5. Token 名: `socket-token` (任意)
6. スコープ: `connections:write` を追加
7. **Generate** → `xapp-...` をコピー

#### Socket Mode 有効化

1. **Settings** → **Socket Mode** を開く
2. **Enable Socket Mode** をオン

#### Event Subscriptions 設定

1. **Features** → **Event Subscriptions** を開く
2. **Enable Events** をオン
3. **Subscribe to bot events** で `app_mention` を追加
4. **Save Changes**

#### Bot Token Scopes 確認

1. **Features** → **OAuth & Permissions** を開く
2. **Scopes** → **Bot Token Scopes** で以下が設定されているか確認:
   - `app_mentions:read`
   - `chat:write`
   - `reactions:write`

#### ワークスペースにインストール

1. **Settings** → **Install App** を開く
2. **Install to Workspace** → 承認
3. **Bot User OAuth Token** (`xoxb-...`) をコピー

### 3. GitHub Personal Access Token 作成

1. https://github.com/settings/tokens を開く
2. **Generate new token** → **Fine-grained tokens**
3. 設定:
   - Token name: `slack-claude-agent`
   - Repository access: 対象リポジトリを選択
   - Permissions:
     - **Contents**: Read and write
     - **Pull requests**: Read and write
     - **Metadata**: Read-only (自動)
4. **Generate token** → `ghp_...` をコピー

---

## GCE インスタンス作成

### 1. GCP Console を開く

https://console.cloud.google.com/

### 2. Compute Engine に移動

1. 左メニュー → **Compute Engine** → **VM instances**
2. 初回の場合は API を有効化（数分かかる）

### 3. インスタンス作成

**CREATE INSTANCE** をクリック

#### 基本設定

| 項目 | 値 |
|------|-----|
| Name | `slack-claude-agent` |
| Region | `asia-northeast1` (東京) |
| Zone | `asia-northeast1-b` |

#### Machine configuration

| 項目 | 値 |
|------|-----|
| Series | `E2` |
| Machine type | `e2-small` (2 vCPU, 2GB メモリ) |

#### Boot disk

1. **CHANGE** をクリック
2. 設定:
   - Operating system: `Ubuntu`
   - Version: `Ubuntu 22.04 LTS x86/64`
   - Boot disk type: `Balanced persistent disk`
   - Size: `10 GB`
3. **SELECT**

#### Identity and API access

| 項目 | 値 |
|------|-----|
| Service account | `Compute Engine default service account` |
| Access scopes | `Allow default access` |

#### Firewall

- ✅ チェックなし（Socket Mode は外部からのアクセス不要）

#### Advanced options

1. **Advanced options** を展開
2. **Management** タブを選択
3. **Automation** → **Startup script** に以下を貼り付け:

```bash
#!/bin/bash
set -e

# Update system
apt-get update

# Install Node.js (for Claude Code CLI)
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs

# Install Git
apt-get install -y git

# Create application user with shell
useradd -r -m -s /bin/bash slackbot || true

# Create application directory
mkdir -p /opt/slack-claude-agent
chown slackbot:slackbot /opt/slack-claude-agent

echo "Startup script complete."
```

4. **CREATE** をクリック

→ インスタンス作成に 1-2 分かかります

---

## SSH 接続とセットアップ

### 1. SSH 接続（ポートフォワーディング付き）

Claude Code CLI の認証にブラウザが必要なため、ポートフォワーディング付きで接続します。

#### ローカル（Mac）から接続

```bash
gcloud compute ssh slack-claude-agent \
  --zone=asia-northeast1-b \
  -- -L 8080:localhost:8080
```

→ ブラウザが開いて認証が求められる場合があります

### 2. Claude Code CLI インストール

SSH 接続後、GCE 上で実行:

```bash
# Claude Code CLI インストール
sudo npm install -g @anthropic-ai/claude-code

# GitHub CLI インストール（PR 作成に必要）
sudo apt-get install -y gh
```

### 3. Claude Code 認証

```bash
# Claude 認証（ブラウザが開く）
claude auth login
```

→ ローカルのブラウザで `http://localhost:8080` が開き、Anthropic にログイン
→ 認証完了後、ターミナルに戻る

### 4. GitHub 認証

```bash
# GitHub CLI 認証
gh auth login
```

対話式で以下を選択:
- What account: `GitHub.com`
- Protocol: `HTTPS`
- Authenticate: `Paste an authentication token`
- Token: （準備した `ghp_...` を貼り付け）

### 5. Git 設定

```bash
# slackbot ユーザーに切り替え
sudo su - slackbot

# Git 設定
git config --global user.name "Bot Name"
git config --global user.email "bot@example.com"

# 元のユーザーに戻る
exit
```

---

## アプリケーションのデプロイ

### 1. リポジトリをクローン

```bash
# slackbot ユーザーに切り替え
sudo su - slackbot

# ホームディレクトリに移動
cd ~

# リポジトリをクローン
git clone https://github.com/YOUR_ORG/slack-claude-agent.git

# ディレクトリに移動
cd slack-claude-agent

# 元のユーザーに戻る
exit
```

### 2. Go のインストール（まだの場合）

```bash
# Go インストール
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz

# PATH 設定
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile
source /etc/profile

# 確認
go version
```

### 3. ビルド

```bash
# slackbot ユーザーで実行
sudo -u slackbot bash -c 'cd /home/slackbot/slack-claude-agent && /usr/local/go/bin/go build -o bin/server ./cmd/server'
```

### 4. 環境変数ファイル作成

```bash
# .env ファイル作成
sudo nano /home/slackbot/slack-claude-agent/.env
```

以下の内容を貼り付け（実際の値に置き換える）:

```env
# Slack
SLACK_BOT_TOKEN=xoxb-YOUR-TOKEN
SLACK_APP_TOKEN=xapp-YOUR-TOKEN

# Workspace
WORKSPACE_PATH=/home/slackbot/workspace

# GitHub
GITHUB_OWNER=your-org
GITHUB_REPO=your-repo
DEFAULT_BRANCH=main

# Commit Author
AUTHOR_NAME=Your Name
AUTHOR_EMAIL=you@example.com
CO_AUTHOR_NAME=Claude
CO_AUTHOR_EMAIL=noreply+claude@anthropic.com

# Claude CLI
CLAUDE_PATH=claude
MAX_CONCURRENT=5
```

保存: `Ctrl+O` → Enter → `Ctrl+X`

### 5. ワークスペースディレクトリ作成

```bash
# slackbot ユーザーで実行
sudo -u slackbot mkdir -p /home/slackbot/workspace

# リポジトリをクローン
sudo -u slackbot bash -c 'cd /home/slackbot/workspace && git clone https://github.com/YOUR_ORG/YOUR_REPO.git'
```

### 6. systemd サービス作成

```bash
# サービスファイル作成
sudo nano /etc/systemd/system/slack-claude-agent.service
```

以下の内容を貼り付け:

```ini
[Unit]
Description=Slack Claude Agent
After=network.target

[Service]
Type=simple
User=slackbot
WorkingDirectory=/home/slackbot/slack-claude-agent
ExecStart=/home/slackbot/slack-claude-agent/bin/server
Restart=always
RestartSec=5
EnvironmentFile=/home/slackbot/slack-claude-agent/.env
Environment="PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin"
Environment="HOME=/home/slackbot"

[Install]
WantedBy=multi-user.target
```

保存: `Ctrl+O` → Enter → `Ctrl+X`

### 7. サービス起動

```bash
# サービス有効化・起動
sudo systemctl daemon-reload
sudo systemctl enable slack-claude-agent
sudo systemctl start slack-claude-agent

# 状態確認
sudo systemctl status slack-claude-agent
```

### 8. ログ確認

```bash
# リアルタイムログ表示
sudo journalctl -u slack-claude-agent -f
```

期待される出力:
```json
{"time":"...","level":"INFO","msg":"starting slack-claude-agent"}
```

---

## 動作確認

### 1. Slack でテスト

Slack で bot をメンション:

```
@bot こんにちは
```

### 2. ログで確認

GCE のログで以下が表示されれば成功:

```json
{"level":"INFO","msg":"handling mention","channel":"...","user":"..."}
```

---

## トラブルシューティング

### サービスが起動しない

```bash
# エラーログ確認
sudo journalctl -u slack-claude-agent -n 50 --no-pager

# 手動実行でエラー確認
sudo -u slackbot bash -c 'cd /home/slackbot/slack-claude-agent && ./bin/server'
```

### Claude Code が動かない

```bash
# slackbot ユーザーで claude コマンド確認
sudo -u slackbot claude --version

# 認証状態確認
sudo -u slackbot claude auth status
```

### GitHub 認証エラー

```bash
# slackbot ユーザーで gh 認証確認
sudo -u slackbot gh auth status

# 再認証
sudo -u slackbot gh auth login
```

---

## 更新方法

### コードを更新してデプロイ

```bash
# SSH 接続
gcloud compute ssh slack-claude-agent --zone=asia-northeast1-b

# 更新
sudo -u slackbot bash -c 'cd /home/slackbot/slack-claude-agent && git pull'

# リビルド
sudo -u slackbot bash -c 'cd /home/slackbot/slack-claude-agent && /usr/local/go/bin/go build -o bin/server ./cmd/server'

# 再起動
sudo systemctl restart slack-claude-agent

# ログ確認
sudo journalctl -u slack-claude-agent -f
```

---

## まとめ

デプロイ完了後、以下が可能になります:

- ✅ Slack から複数スレッドで同時に指示
- ✅ 実装モード・レビューモードの切り替え
- ✅ 24/7 稼働
- ✅ 自動 PR 作成
