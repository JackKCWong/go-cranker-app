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

	"github.com/JackKCWong/go-cranker-connector/connector"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tv42/httpunix"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	unixsockettr := &httpunix.Transport{
		DialTimeout:           100 * time.Millisecond,
		RequestTimeout:        1 * time.Second,
		ResponseHeaderTimeout: 1 * time.Second,
	}

	tlsSkipVerify := &tls.Config{InsecureSkipVerify: true}

	conn := connector.Connector{
		ServiceName:         "myservice",
		ServiceURL:          "http+unix://myservice",
		WSSHttpClient:       &http.Client{Transport: &http.Transport{TLSClientConfig: tlsSkipVerify}},
		ServiceHttpClient:   &http.Client{Transport: unixsockettr},
		ShutdownTimeout:     5 * time.Second,
		RediscoveryInterval: 5 * time.Second,
	}

	crankerWss := os.Args[1]

	err := conn.Connect(func() []string {
		return []string{crankerWss}
	}, 2)
	if err != nil {
		fmt.Printf("Error connecting cranker %s, err: %q", crankerWss, err)
		return
	}

	_ = os.Remove("unixsocket")
	unixsocket, err := net.Listen("unix", "unixsocket")
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
		conn.Shutdown()

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = srv.Close()
		_ = unixsocket.Close()
		log.Info().Msg("shutdown finished")
	}()

	unixsockettr.RegisterLocation("myservice", "unixsocket")

	http.HandleFunc("/hello", func(rw http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(rw, "world\n")
	})

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(rw, "got: %s", r.URL.Path)
	})

	if err := srv.Serve(unixsocket); err != nil {
		log.Error().AnErr("err", err).Msg("cannot serve on unixsocket")
	}

	wg.Wait()
}
