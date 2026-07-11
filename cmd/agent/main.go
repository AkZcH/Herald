package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		sendHeartbeat(cfg, "")
	}
}