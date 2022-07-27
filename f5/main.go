package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yukinying/f5"
)

func main() {
	ctx := context.Background()
	flag.Parse()
	// initialize.
	r, err := f5.New(flag.Args()...)
	if err != nil {
		log.Fatalf("cannot create f5: %v", err)
	}
	// start the program.
	if err := r.Start(ctx); err != nil {
		log.Fatalf("cannot run: %v", err)
	}
	// listen for F5 or space key.
	go r.ListenForKeys(ctx)
	// wait for Ctrl-C, etc.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-done
	// close the runner.
	r.Close()
}
