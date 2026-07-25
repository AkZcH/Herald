package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type configCache struct {
	data            sync.Map
	lastVersionID   string
	mu              sync.RWMutex
}

func (c *configCache) set(key string, value any) {
	c.data.Store(key, value)
}

func (c *configCache) get(key string) (any, bool) {
	return c.data.Load(key)
}

func (c *configCache) setLastVersionID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastVersionID = id
}

func (c *configCache) getLastVersionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastVersionID
}

type agentConfig struct {
	agentID        string
	namespace      string
	controlPlaneURL string
	port           string
	labels         map[string]string
}

func loadConfig() agentConfig {
	cfg := agentConfig{
		agentID:        os.Getenv("HERALD_AGENT_ID"),
		namespace:      os.Getenv("HERALD_NAMESPACE"),
		controlPlaneURL: os.Getenv("HERALD_CONTROL_PLANE_URL"),
		port:           os.Getenv("HERALD_AGENT_PORT"),
	}

	if cfg.agentID == "" {
		hostname, _ := os.Hostname()
		cfg.agentID = fmt.Sprintf("agent-%s", hostname)
	}
	if cfg.namespace == "" {
		cfg.namespace = "default"
	}
	if cfg.controlPlaneURL == "" {
		cfg.controlPlaneURL = "http://localhost:7700"
	}
	if cfg.port == "" {
		cfg.port = "8800"
	}

	cfg.labels = parseLabels(os.Getenv("HERALD_LABELS"))
	return cfg
}

func parseLabels(raw string) map[string]string {
	labels := map[string]string{}
	if raw == "" {
		return labels
	}
	for _, pair := range splitComma(raw) {
		parts := splitEqual(pair)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}
	return labels
}

func splitComma(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func splitEqual(s string) []string {
	for i, c := range s {
		if c == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func register(cfg agentConfig) error {
	body, _ := json.Marshal(map[string]any{
		"agent_id":  cfg.agentID,
		"namespace": cfg.namespace,
		"labels":    cfg.labels,
	})

	resp, err := http.Post(
		fmt.Sprintf("%s/agents/register", cfg.controlPlaneURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register failed: status %d", resp.StatusCode)
	}

	log.Info().
		// Str("agent_id", cfg.agentID).
		Str("namespace", cfg.namespace).
		Any("labels", cfg.labels).
		Msg("registered with control plane")

	return nil
}

func sendHeartbeat(cfg agentConfig, lastVersionID string) {
	body, _ := json.Marshal(map[string]any{
		"agent_id":   cfg.agentID,
		"version_id": lastVersionID,
	})

	resp, err := http.Post(
		fmt.Sprintf("%s/heartbeat", cfg.controlPlaneURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		log.Warn().Err(err).Msg("heartbeat failed")
		return
	}
	defer resp.Body.Close()
}

func connectSSE(cfg agentConfig, cache *configCache) {
	labelsJSON, _ := json.Marshal(cfg.labels)
	url := fmt.Sprintf("%s/agents/stream?agent_id=%s&namespace=%s&labels=%s",
		cfg.controlPlaneURL, cfg.agentID, cfg.namespace, url.QueryEscape(string(labelsJSON)))

	for {
		resp, err := http.Get(url)
		if err != nil {
			log.Warn().Err(err).Msg("SSE connection failed, retrying in 2s")
			time.Sleep(2 * time.Second)
			continue
		}

		log.Info().Msg("SSE connected to control plane")

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			var event map[string]any
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			eventType, _ := event["type"].(string)

			switch eventType {
			case "connected":
				log.Info().Msg("SSE handshake complete")
			case "config_push":
				applyConfigPush(cfg, cache, event)
			}
		}

		resp.Body.Close()
		log.Warn().Msg("SSE connection dropped, reconnecting in 2s")
		time.Sleep(2 * time.Second)
	}
}

func applyConfigPush(cfg agentConfig, cache *configCache, event map[string]any) {
	versionID, _ := event["version_id"].(string)

	if versionID != "" && versionID == cache.getLastVersionID() {
		log.Debug().Str("version_id", versionID).Msg("duplicate push ignored")
		return
	}

	delta, ok := event["delta"].(map[string]any)
	if !ok {
		return
	}

	for key, value := range delta {
		cache.set(key, value)
		log.Info().Str("key", key).Any("value", value).Msg("config applied")
	}

	if versionID != "" {
		cache.setLastVersionID(versionID)
		sendAck(cfg, versionID)
	}
}

func sendAck(cfg agentConfig, versionID string) {
	body, _ := json.Marshal(map[string]string{
		"agent_id":   cfg.agentID,
		"version_id": versionID,
	})

	resp, err := http.Post(
		fmt.Sprintf("%s/ack", cfg.controlPlaneURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		log.Warn().Err(err).Msg("ack failed")
		return
	}
	defer resp.Body.Close()
	log.Info().Str("version_id", versionID).Msg("ack sent")
}

func startLocalServer(cfg agentConfig, cache *configCache) {
	mux := http.NewServeMux()

	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/config/")
		if key == "" {
			http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
			return
		}

		value, ok := cache.get(key)
		if !ok {
			http.Error(w,
				fmt.Sprintf(`{"error":"key %q not found in local cache"}`, key),
				http.StatusNotFound,
			)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"key":        key,
			"value":      value,
			"version_id": cache.getLastVersionID(),
			"source":     "local_cache",
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":   "ok",
			"agent_id": cfg.agentID,
		})
	})

	addr := fmt.Sprintf(":%s", cfg.port)
	log.Info().Str("port", cfg.port).Msg("agent local server starting")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal().Err(err).Msg("local server failed")
	}
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	cfg := loadConfig()

	log.Logger = log.With().Str("agent_id", cfg.agentID).Logger()
	log.Info().Str("control_plane", cfg.controlPlaneURL).Msg("agent starting")

	for {
		if err := register(cfg); err != nil {
			log.Warn().Err(err).Msg("registration failed, retrying in 2s")
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}

	cache := &configCache{}

	go connectSSE(cfg, cache)
	go startLocalServer(cfg, cache)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		sendHeartbeat(cfg, cache.getLastVersionID())
	}
}