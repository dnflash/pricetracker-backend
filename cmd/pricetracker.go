package main

import (
	"context"
	"net/http"
	"pricetracker/internal/client"
	"pricetracker/internal/database"
	"pricetracker/internal/logger"
	"pricetracker/internal/server"
	"time"
)

func main() {
	dbURI := "mongodb://localhost:27017"
	serverAddr := "localhost:8888"
	appLogger := logger.NewLogger(true, true, true)

	appLogger.Info("Connecting to DB at", dbURI)
	dbConn, err := database.ConnectDB(dbURI)
	if err != nil {
		appLogger.Error("Error when connecting to database", err)
		panic(err)
	}
	defer func() {
		if err := dbConn.Disconnect(context.Background()); err != nil {
			appLogger.Error("Error when disconnecting from database", err)
		}
	}()

	srv := server.Server{
		DB: database.Database{Database: dbConn.Database(database.Name)},
		Client: client.Client{
			Client: &http.Client{Timeout: 15 * time.Second},
			Logger: appLogger,
		},
		Logger: appLogger,
	}

	go srv.FetchDataInInterval(time.NewTicker(60 * time.Second))

	httpSrv := &http.Server{
		Handler:      srv.Router(),
		Addr:         serverAddr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	appLogger.Info("Serving on", httpSrv.Addr)
	panic(httpSrv.ListenAndServe())
}
