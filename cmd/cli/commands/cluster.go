package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

type clusterStatusResponse struct {
	NodeID   string `json:"node_id"`
	Role     string `json:"role"`
	Term     int64  `json:"term"`
	LeaderID string `json:"leader_id"`
}

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Manage the Herald cluster",
}

var clusterInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap the Herald cluster",
	RunE:  runClusterInit,
}

var clusterStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster health and node states",
	RunE:  runClusterStatus,
}

func init() {
	RootCmd.AddCommand(clusterCmd)
	clusterCmd.AddCommand(clusterInitCmd)
	clusterCmd.AddCommand(clusterStatusCmd)
}

func runClusterInit(cmd *cobra.Command, args []string) error {
	fmt.Println(bold.Render("HERALD v1.0.0"))
	fmt.Println(dim.Render("bootstrapping raft cluster..."))

	resp, err := httpClient.Get(getURL("/health"))
	if err != nil {
		fmt.Printf("%s could not reach control plane at %s\n", crossMark, controlPlaneURL)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("%s control plane reachable at %s\n", checkMark, cyan.Render(controlPlaneURL))
	}

	statusResp, err := httpClient.Get(getURL("/cluster/status"))
	if err != nil {
		return err
	}
	defer statusResp.Body.Close()

	body, err := io.ReadAll(statusResp.Body)
	if err != nil {
		return err
	}

	var status clusterStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return err
	}

	if status.Role == "leader" {
		fmt.Printf("%s leader elected: %s  term=%s\n",
			checkMark,
			cyan.Render(status.LeaderID),
			yellow.Render(fmt.Sprintf("%d", status.Term)),
		)
		fmt.Printf("\n%s\n", green.Render("cluster ready."))
	} else {
		fmt.Printf("%s no leader yet, node is %s\n", bullet, status.Role)
	}

	return nil
}

func runClusterStatus(cmd *cobra.Command, args []string) error {
	resp, err := httpClient.Get(getURL("/cluster/status"))
	if err != nil {
		fmt.Printf("%s could not reach control plane: %s\n", crossMark, err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var status clusterStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return err
	}

	fmt.Printf("\n%s\n\n", bold.Render("Herald Cluster Status"))

	roleStyle := yellow.Render(status.Role)
	if status.Role == "leader" {
		roleStyle = green.Render(status.Role)
	}

	fmt.Printf("  %s  %s\n", dim.Render("node:  "), cyan.Render(status.NodeID))
	fmt.Printf("  %s  %s\n", dim.Render("role:  "), roleStyle)
	fmt.Printf("  %s  %s\n", dim.Render("term:  "), yellow.Render(fmt.Sprintf("%d", status.Term)))
	fmt.Printf("  %s  %s\n", dim.Render("leader:"), cyan.Render(status.LeaderID))
	fmt.Println()

	return nil
}