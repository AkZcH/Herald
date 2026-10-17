package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var statusNamespace string

type agentRow struct {
	AgentID    string            `json:"agent_id"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Status     string            `json:"status"`
	LastSeen   time.Time         `json:"last_seen"`
	VersionID  string            `json:"version_id"`
	ConfigHash string            `json:"config_hash"`
}

type agentsResponse struct {
	Agents []agentRow `json:"agents"`
	Count  int        `json:"count"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show every agent, its health, and the config version it is running",
	RunE:  runStatus,
}

func init() {
	RootCmd.AddCommand(statusCmd)
	statusCmd.Flags().StringVar(&statusNamespace, "namespace", "", "only show agents in this namespace")
}

func runStatus(cmd *cobra.Command, args []string) error {
	path := "/agents"
	if statusNamespace != "" {
		path += "?namespace=" + url.QueryEscape(statusNamespace)
	}

	resp, err := httpClient.Get(getURL(path))
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

	var out agentsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("decode agents: %w", err)
	}

	renderAgents(cmd.OutOrStdout(), out.Agents, time.Now())
	return nil
}

func renderAgents(w io.Writer, agents []agentRow, now time.Time) {
	if len(agents) == 0 {
		fmt.Fprintf(w, "\n%s\n\n", dim.Render("no agents registered"))
		return
	}

	headers := []string{"AGENT", "NAMESPACE", "LABELS", "STATUS", "LAST SEEN", "VERSION", "HASH"}
	rows := make([][]string, 0, len(agents))
	counts := map[string]int{}

	for _, a := range agents {
		counts[a.Status]++
		rows = append(rows, []string{
			a.AgentID,
			a.Namespace,
			formatLabels(a.Labels),
			a.Status,
			formatAge(now.Sub(a.LastSeen)),
			shortID(a.VersionID),
			shortID(a.ConfigHash),
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

	fmt.Fprintf(w, "\n%s\n\n", bold.Render("Herald Agents"))

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
			case 0, 1:
				padded = cyan.Render(padded)
			case 3:
				padded = statusStyle(cell).Render(padded)
			case 2, 4, 5, 6:
				padded = dim.Render(padded)
			}
			line.WriteString(padded)
			line.WriteString("  ")
		}
		fmt.Fprintf(w, "  %s\n", strings.TrimRight(line.String(), " "))
	}

	noun := "agents"
	if len(agents) == 1 {
		noun = "agent"
	}
	fmt.Fprintf(w, "\n  %d %s: %s, %s, %s\n\n",
		len(agents),
		noun,
		green.Render(fmt.Sprintf("%d healthy", counts["HEALTHY"])),
		yellow.Render(fmt.Sprintf("%d stale", counts["STALE"])),
		red.Render(fmt.Sprintf("%d dead", counts["DEAD"])),
	)
}

func statusStyle(status string) interface{ Render(...string) string } {
	switch status {
	case "HEALTHY":
		return green
	case "STALE":
		return yellow
	case "DEAD":
		return red
	}
	return dim
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func shortID(s string) string {
	if s == "" {
		return "-"
	}
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "-"
	}
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}