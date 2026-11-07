package main

import (
	"context"
	"flag"
	"fmt"
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
	overrides := map[string]string{
		"OPENAI_API_KEY": "local-verification-only", "OPENAI_REALTIME_ENDPOINT": "ws://" + *fixtureAddr + "/realtime",
		"TTS_PROVIDER": "voicevox", "VOICEVOX_ENDPOINT": base,
		"MEMORY_STORE_PATH": filepath.Join(dir, "memories.json"), "CONVERSATION_IDLE_TIMEOUT_SECONDS": "0",
		"SWITCHBOT_TOKEN": "", "SWITCHBOT_SECRET": "", "WS_ADDR": *addr, "RTC_ICE_ADVERTISE_IPS": "",
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
