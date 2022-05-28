package main

import (
	"context"
	"encoding/json"
	"io"
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
	runApp()
	time.Sleep(10 * time.Second)
	os.Exit(1)
}

func runApp() error {
	appContext := context.Background()
	logOutput := io.Writer(os.Stdout)
	appLogger := logger.NewLogger(false, false, true, logOutput)

	defer func() {
		if r := recover(); r != nil {
			appLogger.Errorf("APPLICATION CRASHED: %+v", r)
		}
	}()

	config, err := configuration.GetConfig("config.toml")
	if err != nil {
		appLogger.Error("Error getting configuration from config.toml:", err)
		return err
	}

	if config.LogToFile {
		logFile, err := os.OpenFile("pricetracker_backend.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			appLogger.Error("Error opening log file:", err)
			return err
		}
		defer func() {
			if err := logFile.Close(); err != nil {
				appLogger.Error("Error closing log file:", err)
			}
		}()
		logOutput = io.MultiWriter(logOutput, logFile)
	}
	appLogger = logger.NewLogger(config.LogDebug, config.LogInfo, config.LogError, logOutput)

	if config.LogDebug {
		conf, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			appLogger.Error("Error marshalling Config to JSON:", err)
			return err
		}
		appLogger.Debugf("Config:\n%s", conf)
	}

	appLogger.Info("Connecting to DB at", config.DatabaseURI)
	dbConn, err := database.ConnectDB(appContext, config.DatabaseURI)
	if err != nil {
		appLogger.Error("Error connecting to DB:", err)
		return err
	}
	defer func() {
		if err := dbConn.Disconnect(appContext); err != nil {
			appLogger.Error("Error disconnecting from DB:", err)
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

	appLogger.Info("Starting fetcher with interval:", config.FetchDataInterval)
	go srv.FetchDataInInterval(appContext, time.NewTicker(config.FetchDataInterval))

	httpSrv := &http.Server{
		Handler:      srv.Router(),
		Addr:         config.ServerAddress,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	appLogger.Info("Serving on", httpSrv.Addr)
	return httpSrv.ListenAndServe()
}
