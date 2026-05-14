---
description: Feature flag導入時の実装・テストパターンのベストプラクティス
name: feature-flag-implementation
---

Feature flagを使った機能の段階的リリースを行う際の実装パターンを説明します。

## フラグの配置場所

Feature flagで既存動作を保護しつつ新機能を導入する際の書き方パターン。

### パターン1: unless + early return

```ruby
def some_method
  # TODO: NEW_BEHAVIOR フラグの切り替え完了後にこのunless句を削除する
  unless Feature::NEW_BEHAVIOR.enabled?
    # 既存コード
    return
  end

  # 新しいコード（フラグ有効時のみ実行）
end
```

**利点**:
- フラグ削除時に `unless` ブロック全体を削除するだけ
- 新コードが明確に分離される

### パターン2: if-else（新旧を明示的に分岐）

```ruby
def some_method
  # TODO: NEW_BEHAVIOR フラグの切り替え完了後にelse側を削除する
  if Feature::NEW_BEHAVIOR.enabled?
    # 新しいコード
  else
    # 既存コード
  end
end
```

**利点**:
- 新旧の動作が並列で見やすい
- 両方のパスが明示的

## テスト構造のパターン

**推奨**: デフォルトでフラグON、既存テストを「フラグOFF」コンテキストに移動

```ruby
describe '...' do
  before { allow(Feature::NEW_BEHAVIOR).to receive(:enabled?).and_return(true) }

  # 新しい動作のテストケースをここに記述

  # TODO: NEW_BEHAVIOR フラグの切り替え完了後にコンテキストごと削除する
  context 'NEW_BEHAVIORフラグが無効の場合' do
    before { allow(Feature::NEW_BEHAVIOR).to receive(:enabled?).and_return(false) }

    # 既存のテストケースをここに移動
  end
end
```

**理由**:
- 将来のクリーンアップが容易: フラグ削除時に「フラグが無効の場合」コンテキストを削除するだけ
- デフォルトが新動作: 最終的に残したい動作がデフォルトになる
- 既存動作の保証: 既存テストをそのまま移動することで動作保証を維持
