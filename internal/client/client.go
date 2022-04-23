package client

import (
	"io"
	"net/http"
)

type Client struct {
	*http.Client
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

func newGetRequest(url string, body io.Reader) (*http.Request, error) {
	r, err := http.NewRequest(http.MethodGet, url, body)
	if err != nil {
		return nil, err
	}

	setDefaultRequestHeader(r)

	return r, nil
}

func setDefaultRequestHeader(r *http.Request) {
	r.Header.Set("User-Agent", "Mozilla/5.0")
	r.Header.Set("Accept", "*/*")
	r.Header.Set("Accept-Language", "en-US")
}
