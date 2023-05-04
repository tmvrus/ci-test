package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/fkocharli/metricity/internal/config"
	"github.com/fkocharli/metricity/internal/filewriter"
	"github.com/fkocharli/metricity/internal/handlers"
	"github.com/fkocharli/metricity/internal/repositories"
	"github.com/fkocharli/metricity/internal/server"
	"github.com/fkocharli/metricity/internal/storage/dbstorage"
	"github.com/fkocharli/metricity/internal/storage/filestorage"
	"github.com/fkocharli/metricity/internal/storage/memorystorage"
)

func main() {
	cfg, err := config.NewConfig("server")
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	serverCtx, serverCancel := context.WithCancel(context.Background())
	filerCtx, filerCancel := context.WithCancel(context.Background())
	group := sync.WaitGroup{}

	var storager repositories.Storager

	if cfg.ServerConfig.DBDSN != "" {
		db, err := sql.Open("postgres", cfg.ServerConfig.DBDSN)
		if err != nil {
			log.Printf("Unable to connect to DB. Error: %v", err)
		}
		defer db.Close()

		dbrepo, err := dbstorage.NewRepository(db)
		if err != nil {
			panic(err)
		}

		storager = repositories.NewStorager(dbrepo, nil, cfg.ServerConfig.Key)

		// if cfg.ServerConfig.StoreFile != "" && cfg.ServerConfig.Restore {
		// 	fileRepo, err := filestorage.NewRepository(cfg.ServerConfig.StoreFile, cfg.ServerConfig.StoreInterval)
		// 	if err != nil {
		// 		log.Printf("Error creating file repo: %v\n", err)
		// 	}
		// 	wr := filewriter.New(fileRepo, storager.Repo, cfg.ServerConfig.StoreInterval, cfg.ServerConfig.Restore)
		// 	wr.Load()
		// }
	} else {
		memRepo := memorystorage.NewRepository()
		fileRepo, err := filestorage.NewRepository(cfg.ServerConfig.StoreFile, cfg.ServerConfig.StoreInterval)
		if err != nil {
			log.Printf("Error creating file repo: %v\n", err)
		}

		storager = repositories.NewStorager(memRepo, fileRepo, cfg.ServerConfig.Key)
		if fileRepo != nil {
			defer storager.FileRepo.FileReaderClose()
			defer storager.FileRepo.FileWriterClose()
			wr := filewriter.New(fileRepo, storager.Repo, cfg.ServerConfig.StoreInterval, cfg.ServerConfig.Restore)

			group.Add(1)
			go func() {
				defer group.Done()
				if err := wr.Run(filerCtx); err != nil {
					log.Printf("filewriter run error: %v", err)
					filerCancel()
				}
			}()
		}
	}

	handler := handlers.NewHandler(storager)

	serv := server.New(cfg.ServerConfig.Address, handler.Mux)

	group.Add(1)

	go func() {
		defer group.Done()
		if err := serv.Run(serverCtx); err != nil {
			log.Printf("server run error: %v", err)
			serverCancel()
		}
	}()

	select {
	case <-serverCtx.Done():
		log.Println("service stops via context")
		os.Exit(0)
	case sig := <-waitExitSignal():
		log.Printf("service stoped by signal: %v", sig)

		log.Printf("Shutting down server\n")
		serverCancel()
		log.Printf("Server shuted down\n")
		log.Printf("Trying to save to file\n")
		filerCancel()
	}

	group.Wait()
}

func waitExitSignal() chan os.Signal {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	return sigs
}
