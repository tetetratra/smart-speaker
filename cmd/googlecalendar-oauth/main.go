package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"smart-speaker/internal/oauth/googlecalendar"
)

func main() {
	addr := flag.String("addr", ":3939", "OAuthコールバックを受け付けるアドレス")
	timeout := flag.Duration("timeout", 10*time.Minute, "認証待機のタイムアウト")
	flag.Parse()

	state, err := googlecalendar.RandomState()
	if err != nil {
		log.Fatalf("state生成に失敗しました: %v", err)
	}
	authURL, err := googlecalendar.AuthURL(state)
	if err != nil {
		log.Fatalf("認可URL生成に失敗しました: %v", err)
	}

	fmt.Println("以下のURLをブラウザで開いて認証してください。")
	fmt.Println(authURL)

	if err := googlecalendar.ServeAuthCallback(*addr, *timeout); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			if _, loadErr := googlecalendar.LoadToken(); loadErr == nil {
				log.Println("認証が完了しました。トークンを保存しました。")
				return
			}
		}
		log.Fatalf("OAuthコールバック待機に失敗しました: %v", err)
	}
	log.Println("認証が完了しました。トークンを保存しました。")
}
