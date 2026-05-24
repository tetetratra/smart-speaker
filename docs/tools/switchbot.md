# SwitchBot tools

## 概要
この文書は、SwitchBot tool 群のうち `switchbot_execute_scene` と `hub2_get_environment` の現行実装ベースの概要をまとめたものです。個別デバイス操作ではなく、シーン実行と Hub 2 の環境取得を LLM へ公開する構成です。

## 前提設定
### 共通
- SwitchBot Open API を利用できる `token` と `secret` が必要です。
- 通常起動では `cmd/smart-speaker/main.go` が `SWITCHBOT_TOKEN` と `SWITCHBOT_SECRET` を確認し、どちらかが空の場合は SwitchBot tool 群を登録せずに起動を継続します。
- `registry.New` は `Config.SwitchBotClient` が未指定で、かつ `SwitchBotToken` / `SwitchBotSecret` がどちらも空でない場合に `SwitchBotDeviceMap` から `switchbot.Client` を生成します。
- `SWITCHBOT_TOKEN` / `SWITCHBOT_SECRET` 未設定時は `switchbot_execute_scene` と `hub2_get_environment` は LLM に提示されず、handler も登録されません。

### `hub2_get_environment`
- 登録には有効な `switchbot.Client` が必要です。
- 実行には `hub2` というエイリアスが `SwitchBotDeviceMap` に含まれている必要があります。通常起動では token/secret が揃っていれば tool は登録され、`hub2` alias 未設定は実行時エラーとして返ります。
- `SwitchBotDeviceMap` は JSON オブジェクト文字列です。キーはエイリアス、値は device ID です。

例:

```json
{
  "hub2": "xxxxxxxxxxxxxxxx"
}
```

### `switchbot_execute_scene`
- 登録には有効な `switchbot.Client` が必要です。
- 通常起動では起動時に SwitchBot API から scene 一覧を取得し、その結果が `Config.SwitchBotScenes` として渡されます。
- scene 一覧取得に失敗した場合、`switchbot_execute_scene` だけが登録されません。`hub2_get_environment` は token/secret が揃っていれば登録されます。
- 起動時に取得した `Config.SwitchBotScenes` が 1 件以上必要です。
- scene 名と scene ID のどちらかが空の項目は登録対象から除外されます。
- `Config.SwitchBotScenes` が空、または有効な scene が 0 件の場合は tool 自体が登録されません。

## できること
### `hub2_get_environment`
- Hub 2 の温度、湿度、照度を取得します。
- 引数は取りません。
- 対象デバイスは `hub2` エイリアス固定です。

### `switchbot_execute_scene`
- SwitchBot に登録済みの scene を 1 件実行します。
- 引数 `scene_name` に、起動時に取得した scene 名を完全一致で指定します。
- 利用可能な scene 名は tool definition の description に列挙されます。

## tool 定義
### `hub2_get_environment`
- name: `hub2_get_environment`
- parameters: 空の object
- description: `Hub2の温度・湿度・照度を取得します。`

### `switchbot_execute_scene`
- name: `switchbot_execute_scene`
- parameters:

```json
{
  "type": "object",
  "properties": {
    "scene_name": {
      "type": "string",
      "description": "起動時に取得したSwitchBotシーン名"
    }
  },
  "required": ["scene_name"],
  "additionalProperties": false
}
```

- description: 利用可能な scene 名を埋め込んだ文字列です。

## 返り値と副作用
### `hub2_get_environment`
返り値は次の形式です。

```json
{
  "temperature": "26.1",
  "humidity": "55",
  "light_level": "12"
}
```

- 値は `string` として返ります。
- `temperature` / `humidity` / `light_level` の取得に失敗した場合は `"取得不可"` を返します。
- 副作用はありません。
- 内部では `GET /v1.1/devices/{deviceId}/status` を呼び出します。

### `switchbot_execute_scene`
返り値は SwitchBot API の実行結果をそのまま要約した次の形式です。

```json
{
  "statusCode": 100,
  "message": "success",
  "body": {},
  "http_status": 200,
  "scene_id": "scene-1"
}
```

- `body` の詳細構造は SwitchBot API 応答に依存します。
- 副作用として、対象 scene が実行されます。
- 内部では `POST /v1.1/scenes/{sceneId}/execute` を呼び出します。

## エラーと制約
### `hub2_get_environment`
- client 未設定時は `SwitchBot が設定されていません` を返します。
- `hub2` エイリアスが device map に存在しない場合は `未定義のデバイス名です: hub2` を返します。
- SwitchBot API の HTTP エラーや業務エラーは、そのままエラーとして返します。

### `switchbot_execute_scene`
- client 未設定時は `SwitchBot が設定されていません` を返します。
- `scene_name` 未指定時は、利用可能な scene 一覧付きでエラーを返します。
- 未登録の `scene_name` を指定した場合も、利用可能な scene 一覧付きでエラーを返します。
- scene 名のマッチングは前後空白除去後の完全一致です。

## 不明
- scene 実行後に実世界で何が起きるかは、SwitchBot アプリ側に登録された各 scene の内容に依存します。この文書の参照範囲からは判別できないため不明です。
- `hub2_get_environment` の照度 `lightLevel` の単位や値域は、このコードベースの参照範囲からは不明です。

## 参照元
- [internal/tools/functions/switchbot/scene.go](/internal/tools/functions/switchbot/scene.go)
- [internal/tools/functions/switchbot/hub2.go](/internal/tools/functions/switchbot/hub2.go)
- [internal/tools/functions/switchbot/switchbot.go](/internal/tools/functions/switchbot/switchbot.go)
- [internal/tools/registry/registry.go](/internal/tools/registry/registry.go)
- 旧 docs: `git show HEAD^:docs/8.生活操作ツール群.md`
- SwitchBot Open API: https://github.com/OpenWonderLabs/SwitchBotAPI
