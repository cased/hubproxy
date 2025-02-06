package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var port string

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "testserver",
		Short: "Test server for HubProxy",
		Long:  "A simple HTTP server that logs incoming requests for testing HubProxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&port, "port", "p", "8082", "Port to listen on")

	return cmd
}

func run() error {
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

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
