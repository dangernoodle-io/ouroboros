package app

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"dangernoodle.io/ouroboros/internal/backup"
	"dangernoodle.io/ouroboros/internal/store"
)

// Serve opens the database, builds the MCP server, and runs ServeStdio.
// It blocks until the server exits or a fatal error occurs.
// version is the build-injected version string used in MCP server metadata.
func Serve(version string) error {
	db, err := store.InitDB()
	if err != nil {
		return err
	}

	var bk *backup.Backup // intentionally nil; backupCommit handles nil

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
		log.Printf("server error: %v", err)
		return err
	}
	return nil
}
