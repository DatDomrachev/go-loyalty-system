package main

import (
	"context"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/config"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/repository"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/server"
	"github.com/DatDomrachev/go-loyalty-system/internal/app/wpool"
	"log"
	"os"
	"os/signal"
	"runtime"
)

func main() {

	config, err := config.New()
	if err != nil {
		log.Fatalf("failed to configurate:+%v", err)
	}

	config.InitFlags()
	
	repo, err := repository.New(config.StoragePath, config.DBURL)
	if err != nil {
		log.Fatalf("failed to init repository:+%v", err)
	}

	workersCounter := runtime.NumCPU()

	wp := wpool.New(workersCounter);

	s := server.New(config.Address, config.BaseURL, repo, wp)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		oscall := <-c
		log.Printf("system call:%+v", oscall)
		cancel()
	}()

	if err := s.Run(ctx); err != nil {
		log.Printf("failed to serve:+%v\n", err)
	}

}
