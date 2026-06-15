package hf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestTop(t *testing.T) {
	entries := []wireEntry{
		{
			ID: "2401.00001",
			Paper: wirePaper{
				ID:      "2401.00001",
				Title:   "Test Paper One",
				Upvotes: 10,
				Authors: []wireAuthor{{Name: "Alice"}, {Name: "Bob"}, {Name: "Carol"}, {Name: "Dave"}},
			},
		},
		{
			ID: "2401.00002",
			Paper: wirePaper{
				ID:      "2401.00002",
				Title:   "Test Paper Two",
				Upvotes: 5,
				Authors: []wireAuthor{{Name: "Eve"}},
			},
		},
	}
	body, _ := json.Marshal(entries)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	// override baseURL by calling fetchPapers directly with the test server URL
	papers, err := c.fetchPapers(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(papers) != 2 {
		t.Fatalf("got %d papers, want 2", len(papers))
	}
	p := papers[0]
	if p.ID != "2401.00001" {
		t.Errorf("ID = %q, want 2401.00001", p.ID)
	}
	if p.Title != "Test Paper One" {
		t.Errorf("Title = %q, want Test Paper One", p.Title)
	}
	if p.Upvotes != 10 {
		t.Errorf("Upvotes = %d, want 10", p.Upvotes)
	}
	// only first 3 authors
	if len(p.Authors) != 3 {
		t.Errorf("Authors len = %d, want 3", len(p.Authors))
	}
	if p.Authors[0] != "Alice" {
		t.Errorf("Authors[0] = %q, want Alice", p.Authors[0])
	}
	if p.URL != baseURL+"/papers/2401.00001" {
		t.Errorf("URL = %q, want %s/papers/2401.00001", p.URL, baseURL)
	}
}

func TestAuthorsString(t *testing.T) {
	p := &Paper{Authors: []string{"Alice", "Bob", "Carol"}}
	got := p.AuthorsString()
	want := "Alice, Bob, Carol"
	if got != want {
		t.Errorf("AuthorsString() = %q, want %q", got, want)
	}
}

func TestToPaperFallbackID(t *testing.T) {
	// when paper.ID is empty, fall back to entry.ID
	e := wireEntry{
		ID: "fallback-id",
		Paper: wirePaper{
			ID:    "",
			Title: "No ID Paper",
		},
	}
	p := toPaper(e)
	if p.ID != "fallback-id" {
		t.Errorf("ID = %q, want fallback-id", p.ID)
	}
}
