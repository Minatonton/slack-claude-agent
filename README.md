# slack-claude-agent

Slackでボットをメンションして実装を依頼すると、Vertex AI上のClaudeにコード生成を依頼し、GitHub APIでブランチ作成・コミット・PR作成まで自動で行うシステム。

## アーキテクチャ

```
Slack（ユーザーがボットをメンション）
  → GCE上のGoアプリ（Webhook受信・オーケストレーション）
    → Vertex AI Claude（コード生成）
    → GitHub API（Git Data APIでブランチ・コミット・PR作成）
  → Slack（結果をスレッドに返信）
```

## 技術スタック

- Go 1.22+
- chi (HTTPルーター)
- Vertex AI Claude (anthropic-sdk-go)
- GitHub API (go-github/v60)
- Slack (slack-go/slack)
- GCP Secret Manager
- GCE + Nginx + Let's Encrypt

## セットアップ

### 前提条件

- GCPプロジェクト（Vertex AI Claude有効化済み）
- Slack App
- GitHubリポジトリ + Fine-grained PAT

### 1. Slack App作成

1. [api.slack.com/apps](https://api.slack.com/apps) でApp作成
2. **Event Subscriptions** を有効化し、Request URLに `https://bot.yourdomain.com/slack/events` を設定
3. **Subscribe to bot events** で `app_mention` を追加
4. **OAuth & Permissions** の Bot Token Scopes に `chat:write`, `app_mentions:read` を追加
5. ワークスペースにインストール

### 2. GitHub Fine-grained PAT作成

必要な権限:
- **Contents**: Read and write
- **Pull requests**: Read and write
- **Metadata**: Read-only

### 3. GCPセットアップ

```bash
# Vertex AI Claude APIを有効化
gcloud services enable aiplatform.googleapis.com

# Secret Managerにシークレット登録
echo -n "xoxb-..." | gcloud secrets create slack-bot-token --data-file=-
echo -n "..." | gcloud secrets create slack-signing-secret --data-file=-
echo -n "ghp_..." | gcloud secrets create github-pat --data-file=-
```

### 4. GCEインスタンス作成

```bash
# infra/setup-gce.sh のPROJECT_IDを編集してから実行
chmod +x infra/setup-gce.sh
./infra/setup-gce.sh
```

### 5. 環境変数設定

GCEインスタンス上の `/opt/slack-claude-agent/.env` に以下を設定:

```env
PORT=8080
GCP_PROJECT_ID=your-project-id
GCP_LOCATION=us-east5
CLAUDE_MODEL=claude-sonnet-4-20250514
GITHUB_OWNER=your-org
GITHUB_REPO=your-repo
DEFAULT_BRANCH=main
AUTHOR_NAME=your-github-username
AUTHOR_EMAIL=your-email@example.com
```

Secret Manager を使わない場合は以下も追加:

```env
SLACK_SIGNING_SECRET=...
SLACK_BOT_TOKEN=xoxb-...
GITHUB_PAT=ghp_...
```

### 6. デプロイ

```bash
chmod +x infra/deploy.sh
./infra/deploy.sh
```

### 7. HTTPS設定

GCEインスタンス上で:

```bash
sudo cp /opt/slack-claude-agent/nginx.conf /etc/nginx/sites-available/slack-claude-agent
sudo ln -s /etc/nginx/sites-available/slack-claude-agent /etc/nginx/sites-enabled/
sudo certbot --nginx -d bot.yourdomain.com
sudo systemctl restart nginx
```

## 使い方

Slackでボットをメンションして指示:

```
@claude-agent READMEにAPI仕様のセクションを追加してください
```

ボットが自動で:
1. リポジトリのファイル構成を取得
2. Claudeにコード生成を依頼
3. ブランチ作成・コミット・PR作成
4. PR URLをスレッドに返信
