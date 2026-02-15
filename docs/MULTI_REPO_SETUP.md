# 複数リポジトリサポート - セットアップガイド

このガイドでは、複数のGitHubリポジトリを1つのSlack Claudeエージェントで管理する方法を説明します。

## 概要

複数リポジトリサポートにより、1つのボットインスタンスで複数のリポジトリに対して作業できます。Slackスレッドごとにリポジトリを切り替えることが可能です。

## セットアップ手順

### 1. 環境変数の設定

`infra/.env` ファイル（またはGCEのシステム環境変数）に以下を設定:

```bash
# 複数リポジトリをカンマ区切りで指定
GITHUB_REPOS=myorg/backend:main,myorg/frontend:develop,myorg/mobile:main

# デフォルトリポジトリ（省略可、省略時は最初のリポジトリ）
DEFAULT_GITHUB_REPO=myorg/backend

# デフォルトブランチ（リポジトリでブランチ未指定時）
DEFAULT_BRANCH=main
```

#### リポジトリ指定フォーマット

```
owner/repo:branch
```

- `owner`: GitHubの組織名またはユーザー名
- `repo`: リポジトリ名
- `branch`: ブランチ名（省略可、省略時は `DEFAULT_BRANCH` を使用）

#### 例

```bash
# 3つのリポジトリを登録（それぞれ異なるブランチ）
GITHUB_REPOS=acme/api:main,acme/web:develop,acme/mobile:staging

# backend リポジトリをデフォルトに設定
DEFAULT_GITHUB_REPO=acme/api

# ブランチ未指定時は main を使用
DEFAULT_BRANCH=main
```

### 2. ワークスペースディレクトリの準備

`WORKSPACE_PATH` に指定したディレクトリ配下に、各リポジトリをクローン:

```bash
cd /path/to/workspace

# 各リポジトリをクローン
git clone git@github.com:myorg/backend.git
git clone git@github.com:myorg/frontend.git
git clone git@github.com:myorg/mobile.git
```

ディレクトリ構造:
```
/path/to/workspace/
├── backend/     # myorg/backend
├── frontend/    # myorg/frontend
└── mobile/      # myorg/mobile
```

### 3. GitHub Personal Access Token (PAT) の設定

各リポジトリへのアクセス権限を持つGitHub PATを作成:

1. [GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens](https://github.com/settings/tokens?type=beta)
2. 新しいトークンを作成
3. **Repository access** で対象リポジトリを選択
4. **Permissions** で以下を設定:
   - Contents: Read and write
   - Pull requests: Read and write
   - Metadata: Read-only

環境変数 `GITHUB_TOKEN` に設定するか、`git credential` で認証情報を保存。

### 4. ビルド＆起動

```bash
# ビルド
go build -o bin/server ./cmd/server

# 起動
./bin/server
```

起動時のログで、各リポジトリのRunner初期化を確認:

```json
{"level":"INFO","msg":"initialized runner for repository","repository":"myorg/backend","branch":"main"}
{"level":"INFO","msg":"initialized runner for repository","repository":"myorg/frontend","branch":"develop"}
{"level":"INFO","msg":"initialized runner for repository","repository":"myorg/mobile","branch":"main"}
```

## 使い方

### デフォルトリポジトリで作業開始

新しいスレッドでボットをメンション:

```
@bot README.mdを更新してください
```

ボットはデフォルトリポジトリ（`DEFAULT_GITHUB_REPO`）で作業を開始します。

### リポジトリ一覧の確認

```
@bot repos
```

または

```
@bot リポジトリ
```

出力例:
```
📚 利用可能なリポジトリ:
• myorg/backend (ブランチ: main) 👈 現在のリポジトリ
• myorg/frontend (ブランチ: develop)
• myorg/mobile (ブランチ: main)

リポジトリを切り替えるには: switch owner/repo
```

### リポジトリの切り替え

```
@bot switch myorg/frontend
```

または日本語で:

```
@bot 切り替え myorg/mobile
```

成功すると:
```
🔄 リポジトリを myorg/frontend に切り替えました
```

### 切り替え後の作業

同じスレッド内で切り替え後のリポジトリに対して作業:

```
@bot コンポーネントのスタイルを修正してください
```

この指示は `myorg/frontend` リポジトリで実行されます。

## トラブルシューティング

### リポジトリが見つからない

**エラー**: `❌ リポジトリ myorg/xxx が見つかりません`

**対処方法**:
1. `GITHUB_REPOS` の設定を確認
2. `@bot repos` で利用可能なリポジトリを確認
3. 正しいリポジトリ名で `switch` コマンドを実行

### Runnerが見つからない

**エラー**: `❌ エラー: リポジトリ myorg/xxx のRunnerが見つかりません`

**原因**: 起動時にそのリポジトリのRunner初期化に失敗

**対処方法**:
1. サーバーのログを確認: `sudo journalctl -u slack-claude-agent -f`
2. ワークスペースにリポジトリがクローンされているか確認
3. サーバーを再起動

### ワークスペースにリポジトリがない

**エラー**: Claude実行時にリポジトリが見つからない

**対処方法**:
```bash
cd /path/to/workspace
git clone git@github.com:owner/repo.git
```

クローン後、サーバーを再起動。

## 後方互換性

### レガシー設定（単一リポジトリ）

既存の設定も引き続き動作します:

```bash
# 従来の設定
GITHUB_OWNER=myorg
GITHUB_REPO=myrepo
DEFAULT_BRANCH=main
```

この場合、内部的に以下と同等に扱われます:

```bash
GITHUB_REPOS=myorg/myrepo:main
DEFAULT_GITHUB_REPO=myorg/myrepo
```

### 移行方法

レガシー設定から複数リポジトリ設定への移行:

**変更前** (`infra/.env`):
```bash
GITHUB_OWNER=myorg
GITHUB_REPO=backend
DEFAULT_BRANCH=main
```

**変更後**:
```bash
# GITHUB_OWNER と GITHUB_REPO は削除またはコメントアウト
# GITHUB_OWNER=myorg
# GITHUB_REPO=backend

# 新しい設定
GITHUB_REPOS=myorg/backend:main,myorg/frontend:develop
DEFAULT_GITHUB_REPO=myorg/backend
DEFAULT_BRANCH=main
```

サーバーを再起動して変更を反映。

## ベストプラクティス

### 1. デフォルトリポジトリの選択

最も頻繁に使うリポジトリをデフォルトに設定:

```bash
DEFAULT_GITHUB_REPO=myorg/backend  # 最も使用頻度が高い
```

### 2. ブランチ戦略

各リポジトリで異なるデフォルトブランチを設定可能:

```bash
GITHUB_REPOS=myorg/prod-api:main,myorg/dev-api:develop,myorg/staging-api:staging
```

### 3. ワークスペース整理

リポジトリ名とディレクトリ名を一致させる:

```
/workspace/
├── backend/      # myorg/backend
├── frontend/     # myorg/frontend
└── mobile/       # myorg/mobile
```

これにより、Claudeがリポジトリを自動的に見つけられます。

### 4. スレッド管理

- プロジェクトごとに別スレッドを使用
- スレッドの最初でリポジトリを切り替え
- 長期プロジェクトでは定期的に `repos` で確認

## 参考

- [メインREADME](../README.md)
- [デプロイガイド](./DEPLOYMENT.md)
- [環境変数サンプル](../infra/.env.example)
