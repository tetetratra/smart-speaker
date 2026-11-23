// switchbot-webhook は、Webhook送信先を明示的に照会・登録・解除する。
// アプリの起動・再起動ではSwitchBot側の設定を変更しない。
package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tetetratra/smart-speaker/internal/plugins/switchbot"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || (args[0] != "query" && args[0] != "register" && args[0] != "delete") || (args[0] == "query" && len(args) != 1) || (args[0] != "query" && len(args) != 2) {
		return fmt.Errorf("使い方: switchbot-webhook query | register|delete https://<Funnelのホスト名>:8443")
	}
	token, secret := os.Getenv("SWITCHBOT_TOKEN"), os.Getenv("SWITCHBOT_SECRET")
	if strings.TrimSpace(token) == "" || strings.TrimSpace(secret) == "" {
		return fmt.Errorf("SWITCHBOT_TOKEN と SWITCHBOT_SECRET を設定してください")
	}
	client := switchbot.NewSwitchbotClient(token, secret)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var callbackURL string
	if args[0] != "query" {
		u, err := url.Parse(args[1])
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("パスを含まないHTTPSの公開URLを指定してください")
		}
		webhookToken := strings.TrimSpace(os.Getenv("SWITCHBOT_WEBHOOK_TOKEN"))
		if !switchbot.ValidWebhookToken(webhookToken) {
			return fmt.Errorf("SWITCHBOT_WEBHOOK_TOKEN をアプリと同じ値で設定してください")
		}
		u.Path = "/" + webhookToken
		callbackURL = u.String()
	}
	return apply(ctx, client, args[0], callbackURL, os.Stdout)
}

type webhookClient interface {
	WebhookURLs(context.Context) ([]string, error)
	RegisterWebhook(context.Context, string) error
	DeleteWebhook(context.Context, string) error
}

// apply は照会結果を確認してから変更する。受信アプリの起動処理からは呼ばない。
func apply(ctx context.Context, client webhookClient, action, callbackURL string, out io.Writer) error {
	urls, err := client.WebhookURLs(ctx)
	if err != nil {
		return fmt.Errorf("Webhookの照会に失敗しました: %w", err)
	}
	switch action {
	case "query":
		// URLには秘密が含まれるので、出力を公開ログへ貼らない。
		for _, u := range urls {
			fmt.Fprintln(out, u)
		}
	case "register":
		if slices.Contains(urls, callbackURL) {
			fmt.Fprintln(out, "同じ送信先が登録済みです")
			return nil
		}
		if len(urls) != 0 {
			return fmt.Errorf("別のWebhookが登録済みのため変更しません。queryで確認してください")
		}
		if err := client.RegisterWebhook(ctx, callbackURL); err != nil {
			return fmt.Errorf("Webhookの登録に失敗しました: %w", err)
		}
		fmt.Fprintln(out, "Webhookを登録しました")
	case "delete":
		if !slices.Contains(urls, callbackURL) {
			return fmt.Errorf("指定した送信先は登録されていません。変更しません")
		}
		if err := client.DeleteWebhook(ctx, callbackURL); err != nil {
			return fmt.Errorf("Webhookの解除に失敗しました: %w", err)
		}
		fmt.Fprintln(out, "Webhookを解除しました")
	}
	return nil
}
