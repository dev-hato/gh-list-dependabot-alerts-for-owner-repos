# gh-list-dependabot-alerts-for-owner-repos

Organizationやユーザーが所有する全リポジトリの Dependabot alerts（Dependabot による脆弱性アラート）の一覧取を得する GitHub CLI 拡張機能です。

## できること

- `--org` を指定すると、そのOrganization配下の全リポジトリの Dependabot alerts をまとめて取得します。
- `--username` を指定すると、そのユーザーが所有する全リポジトリの Dependabot alerts を取得します。

## 必要な環境

- `gh` コマンドがインストール済みで、`gh auth login` などで認証済みであること。
- 取得対象のOrganization・リポジトリの Dependabot alerts を閲覧できる権限を持つアカウントで認証していること。

## インストール

```sh
gh extension install dev-hato/gh-list-dependabot-alerts-for-owner-repos
```

## 使い方

組織配下の全リポジトリのアラートを取得する場合:

```sh
gh list-dependabot-alerts-for-owner-repos --org <組織名>
```

ユーザーが所有する全リポジトリのアラートを取得する場合:

```sh
gh list-dependabot-alerts-for-owner-repos --username <ユーザー名>
```

`--org` と `--username` はどちらか一方を指定してください。両方とも空のまま実行するとエラーになります。

### オプション

| オプション   | 説明             |
|--------------|------------------|
| `--org`      | 対象の組織名     |
| `--username` | 対象のユーザー名 |

## 出力例

取得結果は、以下のような JSON 配列として標準出力に出力されます。

```json
[
  {
    "number": 3,
    "state": "open",
    "dependency": {
      "package": {
        "ecosystem": "npm",
        "name": "lodash"
      },
      "manifest_path": "package-lock.json",
      "scope": "runtime"
    },
    "security_advisory": {
      "summary": "Prototype Pollution in lodash",
      "severity": "high"
    },
    "html_url": "https://github.com/octocat/Hello-World/security/dependabot/3",
    "created_at": "2026-05-19T21:32:23Z",
    "repository": {
      "full_name": "octocat/Hello-World"
    }
  }
]
```

## 開発

最初に手元のコードを拡張機能としてインストールします:

```sh
gh extension install .
```

その後、コードを変更して動作確認する際は都度以下のコマンドを実行します:

```sh
go build && gh list-dependabot-alerts-for-owner-repos <引数>
```

## 補足: 待機時間について

実行中、標準エラー出力に `Call <path>` や `Wait <時間>` といったログが流れます。
これは API を呼び出すごとにバックオフ処理によって短い待機を挟んでいるためです。
対象リポジトリ数が多いほど、実行完了までに時間がかかります。
