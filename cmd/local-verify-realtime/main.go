package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/localverify"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "検証アプリのアドレス")
	fixtureAddr := flag.String("fixture-addr", "127.0.0.1:18082", "偽API・デバッグ経路のアドレス")
	flag.Parse()
	for _, value := range []string{*addr, *fixtureAddr} {
		host, _, err := net.SplitHostPort(value)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			log.Fatal("検証サーバーはループバックアドレスだけで起動できます")
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	_, handler := localverify.New(ctx)
	server := &http.Server{Addr: *fixtureAddr, Handler: handler}
	listener, err := net.Listen("tcp", *fixtureAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()
	go server.Serve(listener)
	dir, err := os.MkdirTemp("", "smart-speaker-verify-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	base := "http://" + *fixtureAddr
	// 検証用の設定を一時ディレクトリに書き出し、リポジトリのconfig.yamlやdataを使わないようにする。
	configPath := filepath.Join(dir, "config.yaml")
	config := fmt.Sprintf(`server:
  ws_addr: %q
openai:
  realtime:
    endpoint: %q
conversation:
  idle_timeout_seconds: 0
memory:
  store_path: %q
tts:
  provider: voicevox
  voicevox:
    endpoint: %q
`, *addr, "ws://"+*fixtureAddr+"/realtime", filepath.Join(dir, "memories.json"), base)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		log.Fatal(err)
	}
	overrides := map[string]string{
		"OPENAI_API_KEY": "local-verification-only", app.ConfigPathEnv: configPath,
		"SWITCHBOT_TOKEN": "", "SWITCHBOT_SECRET": "", "RTC_ICE_ADVERTISE_IPS": "",
		"GOOGLE_CLIENT_ID": "", "GOOGLE_CLIENT_SECRET": "", "ELEVENLABS_API_KEY": "",
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := overrides[key]; !ok {
			env = append(env, entry)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	command := exec.CommandContext(ctx, "go", "run", "./cmd/smart-speaker")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGTERM) }
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	fmt.Printf("検証画面: http://%s\n偽発話: POST %s/debug/utterance {\"text\":\"こんにちは\"}\n外部APIは使いません。音声合成は確認用の短い音です。\n", *addr, base)
	if err = command.Run(); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
