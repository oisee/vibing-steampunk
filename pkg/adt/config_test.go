package adt

import (
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		username string
		password string
		opts     []Option
		want     *Config
	}{
		{
			name:     "default config",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     nil,
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "EN",
				SessionType: SessionStateful,
				Timeout:     30 * time.Second,
			},
		},
		{
			name:     "with custom client",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithClient("100")},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "100",
				Language:    "EN",
				SessionType: SessionStateful,
				Timeout:     30 * time.Second,
			},
		},
		{
			name:     "with custom language",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithLanguage("DE")},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "DE",
				SessionType: SessionStateful,
				Timeout:     30 * time.Second,
			},
		},
		{
			name:     "with insecure skip verify",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithInsecureSkipVerify()},
			want: &Config{
				BaseURL:            "https://sap.example.com:44300",
				Username:           "testuser",
				Password:           "testpass",
				Client:             "001",
				Language:           "EN",
				InsecureSkipVerify: true,
				SessionType:        SessionStateful,
				Timeout:            30 * time.Second,
			},
		},
		{
			name:     "with stateless session",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithSessionType(SessionStateless)},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "EN",
				SessionType: SessionStateless,
				Timeout:     30 * time.Second,
			},
		},
		{
			name:     "with custom timeout",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts:     []Option{WithTimeout(60 * time.Second)},
			want: &Config{
				BaseURL:     "https://sap.example.com:44300",
				Username:    "testuser",
				Password:    "testpass",
				Client:      "001",
				Language:    "EN",
				SessionType: SessionStateful,
				Timeout:     60 * time.Second,
			},
		},
		{
			name:     "with multiple options",
			baseURL:  "https://sap.example.com:44300",
			username: "testuser",
			password: "testpass",
			opts: []Option{
				WithClient("200"),
				WithLanguage("FR"),
				WithInsecureSkipVerify(),
				WithTimeout(120 * time.Second),
			},
			want: &Config{
				BaseURL:            "https://sap.example.com:44300",
				Username:           "testuser",
				Password:           "testpass",
				Client:             "200",
				Language:           "FR",
				InsecureSkipVerify: true,
				SessionType:        SessionStateful,
				Timeout:            120 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewConfig(tt.baseURL, tt.username, tt.password, tt.opts...)

			if got.BaseURL != tt.want.BaseURL {
				t.Errorf("BaseURL = %v, want %v", got.BaseURL, tt.want.BaseURL)
			}
			if got.Username != tt.want.Username {
				t.Errorf("Username = %v, want %v", got.Username, tt.want.Username)
			}
			if got.Password != tt.want.Password {
				t.Errorf("Password = %v, want %v", got.Password, tt.want.Password)
			}
			if got.Client != tt.want.Client {
				t.Errorf("Client = %v, want %v", got.Client, tt.want.Client)
			}
			if got.Language != tt.want.Language {
				t.Errorf("Language = %v, want %v", got.Language, tt.want.Language)
			}
			if got.InsecureSkipVerify != tt.want.InsecureSkipVerify {
				t.Errorf("InsecureSkipVerify = %v, want %v", got.InsecureSkipVerify, tt.want.InsecureSkipVerify)
			}
			if got.SessionType != tt.want.SessionType {
				t.Errorf("SessionType = %v, want %v", got.SessionType, tt.want.SessionType)
			}
			if got.Timeout != tt.want.Timeout {
				t.Errorf("Timeout = %v, want %v", got.Timeout, tt.want.Timeout)
			}
		})
	}
}

func TestNewHTTPClient(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := cfg.NewHTTPClient()

	if client == nil {
		t.Error("NewHTTPClient returned nil")
	}
	if client.Jar == nil {
		t.Error("HTTP client should have cookie jar")
	}
	if client.Timeout != cfg.Timeout {
		t.Errorf("HTTP client timeout = %v, want %v", client.Timeout, cfg.Timeout)
	}
}

func TestSessionTypes(t *testing.T) {
	if SessionStateful != "stateful" {
		t.Errorf("SessionStateful = %v, want stateful", SessionStateful)
	}
	if SessionStateless != "stateless" {
		t.Errorf("SessionStateless = %v, want stateless", SessionStateless)
	}
	if SessionKeep != "keep" {
		t.Errorf("SessionKeep = %v, want keep", SessionKeep)
	}
}
