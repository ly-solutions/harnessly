package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"github.com/ly-solutions/harnessly/internal/sameorigin"
)

// browserSocket connects the page's RFB socket to the machine's loopback-only websockify.
func browserSocket(endpoint func(context.Context) (int, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sameorigin.Refuse(w, r) {
			return
		}
		port, err := endpoint(r.Context())
		if err != nil {
			http.Error(w, "Remote browser is unavailable: "+err.Error(), http.StatusBadGateway)
			return
		}
		target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
			},
			FlushInterval: -1,
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
				http.Error(w, "Remote browser disconnected", http.StatusBadGateway)
			},
		}
		proxy.ServeHTTP(w, r)
	}
}
