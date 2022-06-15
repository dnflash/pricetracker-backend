package client

import (
	"io"
	"net/http"
)

type Client struct {
	*http.Client
	FCMKey string
	Logger logger
}

type logger interface {
	Debugf(format string, v ...any)
	Infof(format string, v ...any)
	Errorf(format string, v ...any)
}

func newRequest(method string, url string, body io.Reader) (*http.Request, error) {
	r, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	setDefaultRequestHeader(r)
	return r, nil
}

func setDefaultRequestHeader(r *http.Request) {
	r.Header.Set("User-Agent", "Mozilla/5.0")
	r.Header.Set("Accept", "application/json")
}
