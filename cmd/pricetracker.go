package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/go-redis/redis/v9"
	"io"
	"net/http"
	"os"
	"pricetracker/internal/client"
	"pricetracker/internal/configuration"
	"pricetracker/internal/database"
	"pricetracker/internal/logger"
	"pricetracker/internal/server"
	"runtime/debug"
	"time"
)

func main() {
	_ = runApp()
	time.Sleep(10 * time.Second)
	os.Exit(1)
}

func runApp() error {
	appContext := context.Background()
	logOutput := io.Writer(os.Stdout)
	appLogger := logger.New(logger.LevelInfo, logOutput)

	logFile, errLogFile := os.OpenFile("pricetracker_backend.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	defer func() {
		if errLogFile != nil {
			appLogger.Error("Error opening log file:", errLogFile)
			return
		}
		if err := logFile.Sync(); err != nil {
			appLogger.Error("Error syncing log file:", err)
		}
		if err := logFile.Close(); err != nil {
			appLogger.Error("Error closing log file:", err)
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			appLogger.Errorf("Application crashed, err: %v, stack trace:\n%s", r, debug.Stack())
		}
	}()

	defer func() {
		appLogger.Infof("Exiting...")
	}()

	config, err := configuration.GetConfig("config.toml")
	if err != nil {
		appLogger.Error("Error getting configuration from config.toml:", err)
		return err
	}

	if config.LogToFile {
		if errLogFile != nil {
			return errLogFile
		}
		logOutput = io.MultiWriter(logOutput, logFile)
	}
	appLogger = logger.New(config.LogLevel, logOutput)

	conf, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		appLogger.Error("Error marshalling Config to JSON:", err)
		return err
	}
	appLogger.Infof("Config:\n%s", conf)

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

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxConnsPerHost = 8
	t.MaxIdleConnsPerHost = 8
	t.IdleConnTimeout = 100 * time.Second

	t2 := t.Clone()
	t2.TLSClientConfig = &tls.Config{
		NextProtos: nil,
		//CipherSuites: []uint16{
		//	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		//	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		//	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		//	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		//	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		//	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		//},
		SessionTicketsDisabled: false,
		MaxVersion:             tls.VersionTLS12,
		MinVersion:             tls.VersionTLS12,
		//CurvePreferences:            []tls.CurveID{tls.CurveP384},
		DynamicRecordSizingDisabled: false,
		Renegotiation:               0,
		KeyLogWriter:                nil,
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			appLogger.Error("Error closing Redis client:", err)
		}
	}()
	srv := server.Server{
		DB: database.Database{Database: dbConn.Database(database.Name)},
		Client: client.Client{
			Client: &http.Client{
				Timeout: 10 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
				Transport: t2,
			},
			ShopeeClient: &http.Client{
				Timeout: 10 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
				Transport: t2,
			},
			Redis:  rdb,
			Logger: appLogger,
			FCMKey: config.FCMKey,
		},
		Logger:        appLogger,
		AuthSecretKey: config.AuthSecretKey,
	}

	if !(config.ServerEnabled || config.FetcherEnabled) {
		appLogger.Errorf("No functionality enabled")
		return nil
	}

	if config.FetcherEnabled {
		appLogger.Info("Starting fetcher with interval:", config.FetchDataInterval)
		go srv.FetchDataInInterval(appContext, config.FetchDataInterval)
	}

	if config.ServerEnabled {
		httpSrv := &http.Server{
			Handler:        http.TimeoutHandler(srv.Router(), 15*time.Second, http.StatusText(http.StatusServiceUnavailable)),
			Addr:           config.ServerAddress,
			WriteTimeout:   20 * time.Second,
			ReadTimeout:    15 * time.Second,
			IdleTimeout:    60 * time.Second,
			MaxHeaderBytes: 1024,
		}
		appLogger.Info("Serving on", httpSrv.Addr)
		//if err = httpSrv.ListenAndServeTLS(
		//	"/etc/letsencrypt/live/trackee.xyz/fullchain.pem",
		//	"/etc/letsencrypt/live/trackee.xyz/privkey.pem",
		//); err != nil {
		//	appLogger.Errorf("Error listen and serve TLS: %v", err)
		//}
		return httpSrv.ListenAndServe()
	}
	select {}
}
