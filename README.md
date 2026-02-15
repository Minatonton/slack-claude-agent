# slack-claude-agent

Slack でボットをメンションすると、Claude Code CLI が自動でコード生成・PR作成まで行うエージェント。

## アーキテクチャ

```
Slack（人間がメンション）
  ↓ Socket Mode
GCE の Go アプリ
  ↓ exec("claude --print ...")
Claude Code CLI
  ↓ コード生成・Git操作・PR作成
GitHub
  ↓ 結果を Slack に返信
```

## 特徴

- ✅ **Claude Code CLI**: Anthropic 公式ツールを使用
- ✅ **リアルタイム進捗**: ツール実行状況を Slack でライブ表示
- ✅ **セマフォ制御**: 並行実行数を制限
- ✅ **シンプル**: ji9-agent の設計を参考に実装

## セットアップ

### 1. Claude Code CLI のインストール

```bash
npm install -g @anthropic-ai/claude-code

# 認証（ブラウザが開く）
claude auth login
```

### 2. Slack App 作成

1. [api.slack.com/apps](https://api.slack.com/apps) でApp作成
2. **Settings** → **Basic Information** → **App-Level Tokens** で `connections:write` スコープを持つトークン生成（`xapp-...`）
3. **Settings** → **Socket Mode** を有効化
4. **Features** → **Event Subscriptions** を有効化し、**Subscribe to bot events** で `app_mention` を追加
5. **Features** → **OAuth & Permissions** の Bot Token Scopes に `chat:write`, `app_mentions:read`, `reactions:write` を追加
6. ワークスペースにインストールし、Bot User OAuth Token（`xoxb-...`）を取得

### 3. GitHub PAT 作成

Fine-grained PAT with:
- **Contents**: Read and write
- **Pull requests**: Read and write
- **Metadata**: Read-only

### 4. 環境変数設定

`.env` ファイルを作成（`infra/.env.example` 参照）:

```env
SLACK_BOT_TOKEN=xoxb-...
SLACK_APP_TOKEN=xapp-...
WORKSPACE_PATH=/path/to/workspace
GITHUB_OWNER=your-org
GITHUB_REPO=your-repo
DEFAULT_BRANCH=main
AUTHOR_NAME=Your Name
AUTHOR_EMAIL=you@example.com
CLAUDE_PATH=claude
MAX_CONCURRENT=5
```

### 5. ビルド＆実行

```bash
go build -o bin/server ./cmd/server
./bin/server
```

## GCE デプロイ

### インスタンス作成

```bash
cd infra
# setup-gce.sh の PROJECT_ID を編集
chmod +x setup-gce.sh
./setup-gce.sh
```

### Claude Code CLI 認証

GCE にポートフォワーディング付きで SSH:

```bash
gcloud compute ssh slack-claude-agent --zone=asia-northeast1-b -- -L 8080:localhost:8080
```

GCE 上で認証:

```bash
claude auth login
# ブラウザが開くので認証
```

### デプロイ

```bash
chmod +x deploy.sh
./deploy.sh
```

### Git 設定

```bash
gcloud compute ssh slack-claude-agent --zone=asia-northeast1-b
sudo -u slackbot git config --global user.name "Bot Name"
sudo -u slackbot git config --global user.email "bot@example.com"
```

## 使い方

### 基本的な使い方

Slack でボットをメンション:

```
@bot READMEにセットアップ手順を追加してください
```

ボットが:
1. コード生成
2. ブランチ作成
3. コミット
4. PR作成
5. 結果を Slack に返信

### モード切り替え

**実装モード**（デフォルト）:
```
@bot implement 新しい機能を追加してください
```

**レビューモード**:
```
@bot review このPRをレビューしてください
```

モード切り替え後、同じスレッドで会話を続けることができます。

### セッション管理

- **複数スレッド同時実行**: 最大5スレッドまで並行処理可能（`MAX_CONCURRENT=5`）
- **スレッド毎にセッション管理**: 各 Slack スレッドが独立したセッションとして管理されます
- **Claude セッション継続**: スレッド内で Claude のコンテキストが保持されます
- **セッション終了**: `おわり` または `end` でセッションを明示的に終了

### コマンド

| コマンド | 説明 |
|---------|------|
| `review` / `レビュー` | レビューモードに切り替え |
| `implement` / `実装` | 実装モードに切り替え |
| `おわり` / `end` / `終了` | セッション終了 |

## ログ確認

```bash
sudo journalctl -u slack-claude-agent -f
```

## 参考

- [ji9-agent](https://github.com/kobakaito/ji9-agent) - 本実装の参考元
- [Claude Code Docs](https://code.claude.com/docs/en/headless)
