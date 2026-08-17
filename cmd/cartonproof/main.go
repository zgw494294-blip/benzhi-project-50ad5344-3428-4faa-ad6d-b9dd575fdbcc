package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"cartonproof/internal/review"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	return runArgs(os.Args[1:], ":8080")
}

func runArgs(args []string, address string) error {
	flags := flag.NewFlagSet("cartonproof", flag.ContinueOnError)
	smoke := flags.Bool("smoke", false, "run a bounded in-process workflow")
	serve := flags.Bool("serve", false, "serve the HTTP API until interrupted")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *smoke && *serve {
		return errors.New("--smoke and --serve cannot be used together")
	}
	if *smoke {
		return runSmoke()
	}
	if *serve {
		return runServer(address)
	}
	return runStartup()
}

func runServer(address string) error {
	store := review.NewStore()
	server := &http.Server{
		Addr:              address,
		Handler:           review.NewHandler(store),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("CartonProof listening on %s", server.Addr)
	return server.ListenAndServe()
}

func runStartup() error {
	server := httptest.NewServer(review.NewHandler(review.NewStore()))
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	if err := smokeJSON(client, http.MethodGet, server.URL+"/reviews/startup-check", nil, http.StatusNotFound, nil); err != nil {
		return fmt.Errorf("startup probe: %w", err)
	}
	fmt.Println("startup: ready")
	return nil
}

func runSmoke() error {
	server := httptest.NewServer(review.NewHandler(review.NewStore()))
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}

	var created review.Review
	if err := smokeJSON(client, http.MethodPost, server.URL+"/reviews", createRequest{
		SKU:    "CP-APPLE-12",
		Checks: []string{"ingredients", "allergens", "barcode"},
	}, http.StatusCreated, &created); err != nil {
		return fmt.Errorf("create review: %w", err)
	}
	for _, check := range []string{"ingredients", "allergens", "barcode"} {
		if err := smokeJSON(client, http.MethodPost, server.URL+"/reviews/"+created.ID+"/checks/"+check, verdictRequest{Passed: boolPointer(true)}, http.StatusOK, &review.Review{}); err != nil {
			return fmt.Errorf("record %s: %w", check, err)
		}
	}
	var finalized review.Review
	if err := smokeJSON(client, http.MethodPost, server.URL+"/reviews/"+created.ID+"/finalize", nil, http.StatusOK, &finalized); err != nil {
		return fmt.Errorf("finalize review: %w", err)
	}
	if finalized.Status != review.StatusApproved {
		return fmt.Errorf("finalize review: got status %q", finalized.Status)
	}
	var fetched review.Review
	if err := smokeJSON(client, http.MethodGet, server.URL+"/reviews/"+created.ID, nil, http.StatusOK, &fetched); err != nil {
		return fmt.Errorf("retrieve report: %w", err)
	}
	if fetched.Status != review.StatusApproved || fetched.CompletedAt == nil {
		return errors.New("retrieve report: completed approval missing")
	}
	fmt.Printf("smoke: approved %s\n", fetched.ID)
	return nil
}

type createRequest struct {
	SKU    string   `json:"sku"`
	Checks []string `json:"checks"`
}

type verdictRequest struct {
	Passed *bool   `json:"passed"`
	Note   *string `json:"note,omitempty"`
}

func smokeJSON(client *http.Client, method, endpoint string, body any, expectedStatus int, destination any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, payload)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		return fmt.Errorf("expected HTTP %d, got %d", expectedStatus, response.StatusCode)
	}
	if destination == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return err
	}
	return nil
}

func boolPointer(value bool) *bool {
	return &value
}
