package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	auditNamespace string
	auditLast      int
)

type auditRow struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	Key       string          `json:"key"`
	Namespace string          `json:"namespace"`
	OldValue  json.RawMessage `json:"old_value"`
	NewValue  json.RawMessage `json:"new_value"`
	VersionID string          `json:"version_id"`
	Actor     string          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

type auditResponse struct {
	Entries []auditRow `json:"entries"`
	Count   int        `json:"count"`
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Show the append-only audit log, newest first",
	RunE:  runAudit,
}

func init() {
	RootCmd.AddCommand(auditCmd)
	auditCmd.Flags().StringVar(&auditNamespace, "namespace", "", "only show entries in this namespace")
	auditCmd.Flags().IntVar(&auditLast, "last", 20, "number of most recent entries to show")
}

func runAudit(cmd *cobra.Command, args []string) error {
	q := url.Values{}
	q.Set("last", fmt.Sprintf("%d", auditLast))
	if auditNamespace != "" {
		q.Set("namespace", auditNamespace)
	}

	resp, err := httpClient.Get(getURL("/audit?" + q.Encode()))
	if err != nil {
		fmt.Printf("%s could not reach control plane: %s\n", crossMark, err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("%s control plane returned %d: %s\n", crossMark, resp.StatusCode, strings.TrimSpace(string(body)))
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	var out auditResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("decode audit log: %w", err)
	}

	renderAudit(cmd.OutOrStdout(), out.Entries)
	return nil
}

func renderAudit(w io.Writer, entries []auditRow) {
	if len(entries) == 0 {
		fmt.Fprintf(w, "\n%s\n\n", dim.Render("no audit entries"))
		return
	}

	headers := []string{"TIME", "EVENT", "KEY", "NAMESPACE", "ACTOR", "VERSION", "CHANGE"}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{
			e.CreatedAt.Local().Format("Jan 02 15:04:05"),
			e.EventType,
			e.Key,
			e.Namespace,
			e.Actor,
			shortID(e.VersionID),
			formatChange(e.OldValue, e.NewValue),
		})
	}

	// column widths come from the plain text so ANSI colour codes never skew alignment
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, cell := range r {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	fmt.Fprintf(w, "\n%s\n\n", bold.Render("Herald Audit Log"))

	var hdr strings.Builder
	for i, h := range headers {
		hdr.WriteString(pad(h, widths[i]))
		hdr.WriteString("  ")
	}
	fmt.Fprintf(w, "  %s\n", dim.Render(strings.TrimRight(hdr.String(), " ")))

	for _, r := range rows {
		var line strings.Builder
		for i, cell := range r {
			padded := pad(cell, widths[i])
			switch i {
			case 0, 5, 6:
				padded = dim.Render(padded)
			case 1:
				padded = eventStyle(cell).Render(padded)
			case 2, 3:
				padded = cyan.Render(padded)
			}
			line.WriteString(padded)
			line.WriteString("  ")
		}
		fmt.Fprintf(w, "  %s\n", strings.TrimRight(line.String(), " "))
	}

	fmt.Fprintf(w, "\n  %s\n\n", dim.Render(fmt.Sprintf("showing %d most recent", len(entries))))
}

func eventStyle(event string) interface{ Render(...string) string } {
	switch event {
	case "CONFIG_WRITE":
		return green
	case "ROLLBACK":
		return yellow
	case "SCHEMA_CHANGE":
		return cyan
	}
	return dim
}

// formatChange renders "old -> new" as compact single-line JSON, "(none)" for a first write.
func formatChange(oldV, newV json.RawMessage) string {
	return truncate(compactJSON(oldV)+" -> "+compactJSON(newV), 48)
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "(none)"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}