package main

import (
	"context"
	"net/http"
	"os"
	"pricetracker/internal/client"
	"pricetracker/internal/configuration"
	"pricetracker/internal/database"
	"pricetracker/internal/logger"
	"pricetracker/internal/server"
	"time"
)

func main() {
	appContext := context.Background()
	appLogger := logger.NewLogger(false, false, true)

	config, err := configuration.GetConfig("config.toml")
	if err != nil {
		appLogger.Error("Error getting configuration from config.toml:", err)
		os.Exit(1)
	}

	appLogger = logger.NewLogger(config.LogDebugEnabled, config.LogInfoEnabled, config.LogErrorEnabled)

	appLogger.Debugf("Configuration: %+v", *config)

	appLogger.Info("Connecting to DB at", config.DatabaseURI)
	dbConn, err := database.ConnectDB(appContext, config.DatabaseURI)
	if err != nil {
		appLogger.Error("Error connecting to database:", err)
		panic(err)
	}
	defer func() {
		if err := dbConn.Disconnect(appContext); err != nil {
			appLogger.Error("Error disconnecting from database:", err)
		}
	}()

	srv := server.Server{
		DB: database.Database{Database: dbConn.Database(database.Name)},
		Client: client.Client{
			Client: &http.Client{Timeout: 15 * time.Second},
			FCMKey: config.FCMKey,
			Logger: appLogger,
		},
		Logger:        appLogger,
		AuthSecretKey: config.AuthSecretKey,
	}

	go srv.FetchDataInInterval(appContext, time.NewTicker(config.FetchDataInterval))

	httpSrv := &http.Server{
		Handler:      srv.Router(),
		Addr:         config.ServerAddress,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	appLogger.Info("Serving on", httpSrv.Addr)
	panic(httpSrv.ListenAndServe())
}
