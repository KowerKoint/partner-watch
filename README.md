# Partner Watch

同意した2人のAndroid端末間で、端末状態の確認と遠隔スクリーンショット要求を行う個人利用向けシステム。

## リポジトリ構成

- `android/`: Android 16以上向けKotlinアプリ
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

## 本番運用

`deploy/compose.yaml`はアプリケーションサーバーだけを起動する。TLSはホストですでに稼働しているCaddyが終端し、コンテナのループバック公開ポートへ転送する。

最初に`deploy/.env.example`を`deploy/.env`へコピーし、`PW_PUBLIC_URL`を実際のHTTPS公開URLへ変更する。実際の`.env`はGit管理対象外である。

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

秘密情報、SQLite DB、一時画像、Firebaseサービスアカウント、Android署名鍵はGitへ追加しない。
