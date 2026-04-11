package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/vmvarela/ghoten-oras-backend/backend/oras"
)

func main() {
	ctx := context.Background()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	repository := os.Getenv("OCI_REPOSITORY")
	if repository == "" {
		repository = "ghcr.io/myorg/terraform-state"
	}

	backend, err := oras.New(ctx, oras.Config{
		Repository:  repository,
		Compression: "gzip",
		MaxVersions: 10,
		CredentialFunc: func(_ context.Context, _ string) (oras.Credential, error) {
			return oras.Credential{
				Username: "ghoten",
				Password: token,
			}, nil
		},
	})
	if err != nil {
		log.Fatalf("creating backend: %v", err)
	}
	defer backend.Close()

	workspaces, err := backend.Workspaces(ctx)
	if err != nil {
		log.Fatalf("listing workspaces: %v", err)
	}
	fmt.Println("Workspaces:", workspaces)

	mgr, err := backend.StateMgr(ctx, oras.DefaultStateName)
	if err != nil {
		log.Fatalf("getting state manager: %v", err)
	}

	payload, err := mgr.Get(ctx)
	if err != nil {
		log.Fatalf("reading state: %v", err)
	}
	if payload == nil {
		fmt.Println("No state exists yet.")
	} else {
		fmt.Printf("Current state: %d bytes (MD5: %x)\n", len(payload.Data), payload.MD5)
	}

	state := []byte(`{"version": 4, "serial": 1}`)
	if err := mgr.Put(ctx, state); err != nil {
		log.Fatalf("writing state: %v", err)
	}
	fmt.Println("State written successfully.")

	mgr.WaitForRetention()
}
