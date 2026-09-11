package main

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func registerWebUI(mux *http.ServeMux, distDir string) {
	if distDir == "" {
		distDir = "web/dist"
	}
	absDir, err := filepath.Abs(distDir)
	if err != nil {
		log.Printf("web ui: invalid dist dir: %v", err)
		return
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		log.Printf("web ui: dist dir not found: %s", absDir)
		return
	}
	indexPath := filepath.Join(absDir, "index.html")
	if info, err := os.Stat(indexPath); err != nil || info.IsDir() {
		log.Printf("web ui: index.html not found: %s", indexPath)
		return
	}
	log.Printf("web ui: serve %s", absDir)

	fileServer := http.FileServer(http.Dir(absDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cleanPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "/" {
			http.ServeFile(w, r, indexPath)
			return
		}
		targetPath := filepath.Join(absDir, strings.TrimPrefix(cleanPath, "/"))
		if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// assets や拡張子付きの静的ファイルは SPA フォールバックの対象外にする。
		if strings.HasPrefix(cleanPath, "/assets/") || cleanPath == "/assets" || path.Ext(cleanPath) != "" {
			http.NotFound(w, r)
			return
		}

		// SPA ルーティング向けに index.html を直接返す（URL書き換えしない）。
		http.ServeFile(w, r, indexPath)
	}))
}
