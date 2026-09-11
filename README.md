# Partner Watch

同意した2人のAndroid端末間で、通知転送、端末状態の確認、遠隔スクリーンショット要求を行う個人利用向けシステム。

## リポジトリ構成

- `android/`: Android 16以上向けKotlinアプリ
- `desktop/`: Linux向けGoクライアント（通信コアは将来のWindowsと共用）
- `server/`: Go APIサーバーと管理CLI
- `deploy/`: Docker Composeなどの運用設定
- `docs/`: 技術設計、決定ログ、API仕様

## 開発環境

サーバーとAndroidアプリは独立したNix開発環境を持つ。

### サーバー

```sh
cd server
nix develop
go test ./...
go run ./cmd/partner-watch-server
```

既定では`127.0.0.1:8080`で待ち受ける。`PW_LISTEN_ADDR`で変更できる。

端末登録用のペアと、2台分の一回限り招待コードを作るには次を実行する。

```sh
go run ./cmd/partner-watch-admin pair-create \
  --data-dir ./data \
  --name "Partner Watch" \
  --server-url https://watch.example.com
```

招待コードは既定で15分間有効で、それぞれ一度だけ使用できる。出力には秘密情報が含まれるためログへ保存しない。

### Android

Android Studioで`android/`をプロジェクトとして開く。コマンドラインでは次を実行する。

```sh
cd android
nix develop
./gradlew test
./gradlew assembleDebug
```

Android StudioはCLI用開発シェルと分離している。`android/`で次を実行すると、必要な場合だけStudio一式を取得して起動できる。

```sh
nix run .#android-studio -- .
```

Android SDK、Emulator、API 36のGoogle Play対応x86_64システムイメージはNixで管理され、Nixストア内では読み取り専用になる。StudioのSDK Managerからパッケージを追加・更新せず、flakeを変更して`nix run .#android-studio -- .`で起動し直す。AVD定義と仮想端末データは通常どおり利用者の`~/.android`へ保存される。API 37はコンパイル用プラットフォームとして含め、エミュレータ試験はアプリの最小対象であるAPI 36で行う。

API 36の試験用AVDは次のように作成できる。新しいシステムイメージに`devices.xml`がないという警告が表示されても、`avdmanager list avd`にAVDが表示されれば作成は完了している。

```sh
cd android
nix develop -c avdmanager create avd \
  --name PartnerWatch_API_36 \
  --package 'system-images;android-36;google_apis_playstore;x86_64' \
  --device pixel_6
nix develop -c avdmanager list avd
```

### Linuxデスクトップ

初期対応環境はNixOS・niri/Wayland・swayncである。

```sh
cd desktop
nix develop
go build ./cmd/partner-watch-desktop
```

`config.example.toml`を`~/.config/partner-watch/config.toml`へコピーして編集し、管理CLIで発行した追加端末招待コードを使って登録する。

```sh
partner-watch-desktop enroll
partner-watch-desktop run
```

資格情報は`~/.local/state/partner-watch/state.json`へパーミッション`0600`で保存される。通知転送は設定の`forward_notifications = true`で明示的に有効化する。

flakeパッケージは`desktop/`で`nix build`または`nix run .# -- run`として利用できる。実行時に必要な`grim`と`niri`もパッケージのPATHへ含まれる。

常用時はHome Managerモジュールを推奨する。Nix設定のinputへ`github:KowerKoint/partner-watch?dir=desktop`を追加し、次のように設定する。

```nix
{
  imports = [ inputs.partner-watch.homeManagerModules.default ];
  services.partner-watch-desktop = {
    enable = true;
    serverUrl = "https://partner-watch.example.com";
    deviceName = "My niri PC";
    acceptCaptures = true;
    forwardNotifications = true;
  };
}
```

Home Managerを使わない場合は`inputs.partner-watch.nixosModules.default`をNixOS設定へimportし、上記に加えて`user = "my-user";`を指定する。

設定反映後、サービスを開始する前に一度だけ追加端末招待コードで登録する。Home Manager版では`partner-watch-desktop enroll --invite-code ...`、NixOS版では次の専用ラッパーを使う。

```sh
partner-watch-desktop-enroll --invite-code '<追加端末招待コード>'
systemctl --user start partner-watch-desktop.service
systemctl --user status partner-watch-desktop.service
```

資格情報ファイルがまだ存在しない間、ユーザーサービスは起動しない。

## 本番運用

`deploy/compose.yaml`はアプリケーションサーバーだけを起動する。TLSはホストですでに稼働しているCaddyが終端し、コンテナのループバック公開ポートへ転送する。

最初に`deploy/.env.example`を`deploy/.env`へコピーし、`PW_PUBLIC_URL`を実際のHTTPS公開URLへ変更する。`PW_HOST_PORT`はCaddyが接続するホスト側ポートで、未指定時は`18080`となる。実際の`.env`はGit管理対象外である。

本番とテストを同じホストで動かす場合は、別の環境ファイル、ホスト側ポート、Composeプロジェクト名を使用する。たとえばテスト用の`deploy/.env.test`では`PW_HOST_PORT=18090`を指定し、次のように起動する。

```console
docker compose --project-name partner-watch-test --env-file deploy/.env.test -f deploy/compose.yaml up -d --build
```

Composeプロジェクト名を分けることでコンテナと名前付きデータボリュームも本番から分離される。

Docker Compose環境で招待コードを発行する場合は、サーバーと同じ永続ボリュームを使って管理CLIを実行する。

```sh
docker compose -f deploy/compose.yaml run --rm \
  --entrypoint /partner-watch-admin server \
  pair-create --name "Partner Watch"
```

不要になったペアは、ペアIDを指定して削除できる。確認プロンプトを省略する場合だけ`--yes`を付ける。

ペアIDが不明な場合は、一覧を表示する。

```sh
docker compose -f deploy/compose.yaml run --rm \
  --entrypoint /partner-watch-admin server pair-list
```

```sh
docker compose -f deploy/compose.yaml run --rm \
  --entrypoint /partner-watch-admin server \
  pair-delete --pair-id "PAIR_ID"
```

既存ペアの同じ利用者側へLinuxなどの追加端末を登録する場合は、単回利用の招待コードを発行する。

```sh
docker compose -f deploy/compose.yaml run --rm \
  --entrypoint /partner-watch-admin server \
  device-invite --pair-id "PAIR_ID" --slot 1
```

端末一覧の確認と端末単位の失効は次のように行う。

```sh
docker compose -f deploy/compose.yaml run --rm \
  --entrypoint /partner-watch-admin server \
  device-list --pair-id "PAIR_ID"

docker compose -f deploy/compose.yaml run --rm \
  --entrypoint /partner-watch-admin server \
  device-revoke --device-id "DEVICE_ID"
```

秘密情報、SQLite DB、一時画像、Firebaseサービスアカウント、Android署名鍵はGitへ追加しない。
