package main

import (
	"context"
	"log"
	"os"

	clicmd "github.com/oniharnantyo/tinyroute/internal/cli"
)

func main() {
	if err := clicmd.New().Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
