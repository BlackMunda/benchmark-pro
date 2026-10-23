package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"benchmarkpro/internal/ghclient"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	secret := os.Getenv("WEBHOOK_SECRET")
	token := os.Getenv("GITHUB_TOKEN")
	port := env("PORT", "8080")

	if secret == "" {
		log.Fatal("WEBHOOK_SECRET is required (the secret configured on the GitHub webhook)")
	}
	if token == "" {
		log.Fatal("GITHUB_TOKEN is required (a token with repo:status and public_repo/repo scope)")
	}

	srv := NewServer(secret, ghclient.New(token))
	if wd := os.Getenv("WORKDIR"); wd != "" {
		srv.WorkDir = wd
	}

	http.Handle("/webhook", srv.Handler())
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	addr := ":" + port
	log.Printf("benchmark-pro-server listening on %s (POST /webhook)", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
