// Package oras provides a standalone backend for storing Terraform/OpenTofu
// state as OCI artifacts in container registries using the ORAS protocol.
//
// This package is provider-agnostic: it works with both Terraform and OpenTofu.
// Encryption is intentionally excluded; callers that need state encryption
// should wrap the returned byte slices at the adapter layer.
package oras
