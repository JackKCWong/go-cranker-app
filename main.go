package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/JackKCWong/go-cranker-connector/pkg/config"
	"github.com/JackKCWong/go-cranker-connector/pkg/connector/v1"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tv42/httpunix"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	u := &httpunix.Transport{
		DialTimeout:           100 * time.Millisecond,
		RequestTimeout:        1 * time.Second,
		ResponseHeaderTimeout: 1 * time.Second,
	}

	tlsSkipVerify := &tls.Config{InsecureSkipVerify: true}
	conn := connector.NewConnector(
		&config.RouterConfig{
			TLSClientConfig:   tlsSkipVerify,
			WSHandshakTimeout: 1 * time.Second,
		},
		&config.ServiceConfig{
			HTTPClient: &http.Client{
				Transport: u,
			},
		})

	crankerWss := os.Args[1]

	err := conn.Connect([]string{crankerWss}, 2, "myservice", "http+unix://myservice")
	if err != nil {
		fmt.Printf("Error connecting cranker %s, err: %q", crankerWss, err)
		return
	}

	os.Remove("unixsocket")
	usocket, err := net.Listen("unix", "unixsocket")
	if err != nil {
		log.Error().AnErr("err", err).Msg("cannot listen on unixsocket")
	}

	srv := &http.Server{}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-c
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		log.Info().Msg("shutting down...")
		conn.Shutdown(ctx)

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)

		srv.Close()
		usocket.Close()
		log.Info().Msg("shutdown finished")
	}()

	u.RegisterLocation("myservice", "unixsocket")

	http.HandleFunc("/hello", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(rw, "hello\n")
	})

	if err := srv.Serve(usocket); err != nil {
		log.Error().AnErr("err", err).Msg("cannot serve on unixsocket")
	}

	wg.Wait()
}
