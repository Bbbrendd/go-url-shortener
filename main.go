package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

type Storage interface {
	Save(url string) (string, error)
	Get(id string) (string, error)
	Metrics(id string) (uint64, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	urls    map[string]string
	clicks  map[string]*uint64
	counter uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		urls:   make(map[string]string),
		clicks: make(map[string]*uint64),
	}
}

func (m *MemoryStore) encode(num uint64) string {
	if num == 0 {
		return string(alphabet[0])
	}
	var sb strings.Builder
	length := uint64(len(alphabet))
	for num > 0 {
		sb.WriteByte(alphabet[num%length])
		num /= length
	}
	return sb.String()
}

func (m *MemoryStore) Save(url string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idNum := atomic.AddUint64(&m.counter, 1)
	shortID := m.encode(idNum)

	var initialClicks uint64 = 0
	m.urls[shortID] = url
	m.clicks[shortID] = &initialClicks

	return shortID, nil
}

func (m *MemoryStore) Get(id string) (string, error) {
	m.mu.RLock()
	url, exists := m.urls[id]
	clicksPtr := m.clicks[id]
	m.mu.RUnlock()

	if !exists {
		return "", errors.New("url not found")
	}

	atomic.AddUint64(clicksPtr, 1)
	return url, nil
}

func (m *MemoryStore) Metrics(id string) (uint64, error) {
	m.mu.RLock()
	clicksPtr, exists := m.clicks[id]
	m.mu.RUnlock()

	if !exists {
		return 0, errors.New("url not found")
	}

	return atomic.LoadUint64(clicksPtr), nil
}

type Service struct {
	store Storage
}

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	ShortID string `json:"short_id"`
	URL     string `json:"url"`
}

func (s *Service) HandleShorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	shortID, err := s.store.Save(req.URL)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ShortenResponse{
		ShortID: shortID,
		URL:     fmt.Sprintf("http://%s/%s", r.Host, shortID),
	})
}

func (s *Service) HandleRedirectOrStats(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if strings.HasPrefix(path, "stats/") {
		id := strings.TrimPrefix(path, "stats/")
		clicks, err := s.store.Metrics(id)
		if err != nil {
			http.Error(w, "Short URL not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"short_id": id, "clicks": clicks})
		return
	}

	url, err := s.store.Get(path)
	if err != nil {
		http.Error(w, "Short URL not found", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func main() {
	store := NewMemoryStore()
	svc := &Service{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", svc.HandleShorten)
	mux.HandleFunc("/", svc.HandleRedirectOrStats)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Println("Server listening on :8080...")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server crash: %v\n", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server safely...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited cleanly")
}
