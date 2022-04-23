package server

import (
	"pricetracker/internal/client"
	"pricetracker/internal/database"
)

type Server struct {
	DB     database.Database
	Client client.Client
	Logger logger
}

type logger interface {
	Debug(v ...any)
	Info(v ...any)
	Error(v ...any)
	Debugf(format string, v ...any)
	Infof(format string, v ...any)
	Errorf(format string, v ...any)
}
