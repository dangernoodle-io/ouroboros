package main

import (
	"os"

	"dangernoodle.io/ouroboros/internal/app"
)

var Version = "dev"

func main() {
	os.Exit(app.Run(os.Args[1:], Version))
}
