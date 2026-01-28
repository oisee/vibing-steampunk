// ABOUTME: Integration test helpers for SAP ADT integration tests.
// ABOUTME: Provides getIntegrationClient() for tests needing real SAP connection.

package adt

import (
	"os"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/testutil"
)

// getIntegrationClient creates an ADT client for integration tests.
// Loads credentials from .env file or environment variables.
func getIntegrationClient(t *testing.T) *Client {
	testutil.LoadEnv()

	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")

	if url == "" || user == "" || pass == "" {
		t.Skip("SAP_URL, SAP_USER, SAP_PASSWORD required for integration tests (set in .env or environment)")
	}

	client := os.Getenv("SAP_CLIENT")
	if client == "" {
		client = "001"
	}
	lang := os.Getenv("SAP_LANGUAGE")
	if lang == "" {
		lang = "EN"
	}

	opts := []Option{
		WithClient(client),
		WithLanguage(lang),
		WithTimeout(30 * time.Second),
	}

	if os.Getenv("SAP_INSECURE") == "true" {
		opts = append(opts, WithInsecureSkipVerify())
	}

	return NewClient(url, user, pass, opts...)
}

// getTestUser returns the test user for debugging tests.
func getTestUser(t *testing.T) string {
	testutil.LoadEnv()

	user := os.Getenv("SAP_USER")
	if user == "" {
		t.Skip("SAP_USER required")
	}
	return user
}
