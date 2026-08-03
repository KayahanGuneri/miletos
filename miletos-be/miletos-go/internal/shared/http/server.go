// Package http owns the runtime's shared HTTP transport.
package http

import (
	nethttp "net/http"
	"time"
)

func NewServer(address string, handler nethttp.Handler) *nethttp.Server {
	return &nethttp.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
