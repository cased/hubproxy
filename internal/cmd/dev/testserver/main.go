package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var port string
var socketPath string

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "testserver",
		Short: "Test server for HubProxy",
		Long:  "A simple HTTP server that logs incoming requests for testing HubProxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if socketPath != "" {
				return runUnixSocket(socketPath)
			} else if port != "" {
				return runPort(port)
			}
			return fmt.Errorf("no port or socket specified")
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&port, "port", "p", "", "Port to listen on")
	flags.StringVarP(&socketPath, "unix-socket", "s", "", "Unix socket to listen on")

	return cmd
}

func installHTTPHandler() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received %s request from %s", r.Method, r.RemoteAddr)
		log.Printf("Headers: %v", r.Header)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading body: %v", err)
			http.Error(w, "Error reading body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		log.Printf("Body: %s", string(body))
		w.WriteHeader(http.StatusOK)
	})
}

func runPort(port string) error {
	installHTTPHandler()

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      http.DefaultServeMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Test server listening on :%s", port)
	return srv.ListenAndServe()
}

func runUnixSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	installHTTPHandler()

	srv := &http.Server{
		Handler:      http.DefaultServeMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()

	if err := os.Chmod(socketPath, 0666); err != nil {
		return err
	}

	log.Printf("Test server listening on unix socket: %s", socketPath)
	return srv.Serve(listener)
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
