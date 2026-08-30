# gh-list-dependabot-alerts-for-owner-repos

<!-- textlint-disable ja-technical-writing/ja-no-mixed-period -->

[English](README.md) | 日本語

<!-- textlint-enable ja-technical-writing/ja-no-mixed-period -->

Dependabot alerts（Dependabotによる脆弱性アラート）の一覧を取得するGitHub CLI拡張機能です。
Organizationやユーザーが所有する全リポジトリを対象とします。

## できること

- `--org`を指定すると、そのOrganization配下の全リポジトリのalertsをまとめて取得する。
- `--org`を指定しない場合、認証済みユーザー（`gh`でログインしているアカウント）が所有する全リポジトリのalertsを取得する。

## 必要な環境

- `gh`コマンドがインストール済みで、`gh auth login`などで認証済みであること。
- Dependabot alertsを閲覧できる権限を持つアカウントで認証していること。

## インストール

```sh
gh extension install dev-hato/gh-list-dependabot-alerts-for-owner-repos
```

## 使い方

組織配下の全リポジトリのアラートを取得する場合は、次のコマンドを実行します。

```sh
gh list-dependabot-alerts-for-owner-repos --org <組織名>
```

認証済みユーザーが所有する全リポジトリのアラートを取得する場合は、フラグを付けずに次のコマンドを実行します。

```sh
gh list-dependabot-alerts-for-owner-repos
```

### オプション

| オプション     | 説明                     |
| -------------- | ------------------------ |
| `--org`        | 対象の組織名             |
| `-h`, `--help` | ヘルプを表示して終了する |

## 出力例

取得結果は、次のようなJSON配列として標準出力に出力されます。

<!-- jscpd:ignore-start -->

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

<!-- jscpd:ignore-end -->

## 開発

最初に手元のコードを拡張機能としてインストールします。

```sh
gh extension install .
```

その後、コードを変更して動作確認する際は都度、次のコマンドを実行します。

```sh
go build && gh list-dependabot-alerts-for-owner-repos <引数>
```

## 補足: 待機時間について

実行中、標準エラー出力にAPI呼び出し前の`Call <path>`といったログが流れます。
リクエスト頻度を一定に保つため、呼び出し前に短い待機を挟む場合があります。
認証済みユーザー自身のリポジトリを対象にする場合（`--org`を省略した場合）は、各リポジトリのアラート取得を並列に行いますが、リクエストは全体で同じレート制限を共有します。
対象リポジトリ数が多いほど、実行完了までに時間がかかります。
