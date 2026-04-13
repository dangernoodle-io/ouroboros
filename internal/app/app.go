package app

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
	"dangernoodle.io/ouroboros/internal/store"
)

func Run(args []string, version string) int {
	for _, arg := range args {
		if arg == "--version" || arg == "-v" {
			fmt.Println(version)
			return 0
		}
	}

	if len(args) > 0 && args[0] == "query" {
		runQuery(args[1:])
		return 0
	}
	if len(args) > 0 && args[0] == "items" {
		runItems(args[1:])
		return 0
	}

	db, err := store.InitDB()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	var bk *backup.Backup

	s := buildServer(db, bk, version)

	signal.Ignore(syscall.SIGPIPE)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		db.Close()
		os.Exit(0)
	}()

	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
	return 0
}
