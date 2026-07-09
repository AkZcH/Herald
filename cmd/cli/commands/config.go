package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

var (
	configNamespace string
	configTarget    string
	configTTL       string
	configPushedBy  string
)

type configWriteRequest struct {
	Key             string            `json:"key"`
	Value           any               `json:"value"`
	Namespace       string            `json:"namespace"`
	TTL             string            `json:"ttl,omitempty"`
	TargetingLabels map[string]string `json:"targeting_labels,omitempty"`
	PushedBy        string            `json:"pushed_by"`
}

type configWriteResponse struct {
	VersionID   string         `json:"version_id"`
	Key         string         `json:"key"`
	Namespace   string         `json:"namespace"`
	Status      string         `json:"status"`
	VectorClock map[string]int `json:"vector_clock"`
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Push a config value to the cluster",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)

	configSetCmd.Flags().StringVar(&configNamespace, "namespace", "default", "namespace")
	configSetCmd.Flags().StringVar(&configTarget, "target", "", "label selector e.g. env=prod,region=us")
	configSetCmd.Flags().StringVar(&configTTL, "ttl", "", "TTL e.g. 24h")
	configSetCmd.Flags().StringVar(&configPushedBy, "pushed-by", "cli", "actor name")
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	rawValue := args[1]

	var value any
	if err := json.Unmarshal([]byte(rawValue), &value); err != nil {
		value = rawValue
	}

	req := configWriteRequest{
		Key:       key,
		Value:     value,
		Namespace: configNamespace,
		TTL:       configTTL,
		PushedBy:  configPushedBy,
	}

	body, _ := json.Marshal(req)
	resp, err := httpClient.Post(getURL("/config"), "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("%s could not reach control plane: %s\n", crossMark, err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result configWriteResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("%s unexpected response: %s\n", crossMark, string(respBody))
		return err
	}

	fmt.Printf("\n%s writing to leader %s\n", bullet, cyan.Render(controlPlaneURL))
	fmt.Printf("%s committed  key=%s  version=%s\n",
		checkMark,
		cyan.Render(result.Key),
		dim.Render(result.VersionID),
	)
	fmt.Printf("%s namespace=%s  status=%s\n",
		bullet,
		yellow.Render(result.Namespace),
		green.Render(result.Status),
	)

	if len(result.VectorClock) > 0 {
		clockBytes, _ := json.Marshal(result.VectorClock)
		fmt.Printf("%s vector clock: %s\n", bullet, dim.Render(string(clockBytes)))
	}
	fmt.Println()

	return nil
}