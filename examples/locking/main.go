package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

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
		Repository: repository,
		LockTTL:    10 * time.Minute,
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

	mgr, err := backend.StateMgr(ctx, oras.DefaultStateName)
	if err != nil {
		log.Fatalf("getting state manager: %v", err)
	}

	lockID, err := mgr.Lock(ctx, &oras.LockInfo{
		Operation: "OperationTypeApply",
		Who:       "example@host",
		Version:   "1.0.0",
		Created:   time.Now().UTC(),
	})
	if err != nil {
		log.Fatalf("acquiring lock: %v", err)
	}
	fmt.Printf("Lock acquired: %s\n", lockID)

	// Read-modify-write cycle while holding the lock.
	payload, err := mgr.Get(ctx)
	if err != nil {
		log.Fatalf("reading state: %v", err)
	}
	if payload == nil {
		fmt.Println("No existing state, writing initial state.")
	} else {
		fmt.Printf("Read state: %d bytes\n", len(payload.Data))
	}

	if err := mgr.Put(ctx, []byte(`{"version": 4, "serial": 2}`)); err != nil {
		log.Fatalf("writing state: %v", err)
	}
	fmt.Println("State updated under lock.")

	if err := mgr.Unlock(ctx, lockID); err != nil {
		log.Fatalf("releasing lock: %v", err)
	}
	fmt.Println("Lock released.")
}
