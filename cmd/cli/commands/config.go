package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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

type configGetResponse struct {
	Key         string         `json:"key"`
	Namespace   string         `json:"namespace"`
	Value       any            `json:"value"`
	VersionID   string         `json:"version_id"`
	Status      string         `json:"status"`
	VectorClock map[string]int `json:"vector_clock"`
	PushedBy    string         `json:"pushed_by"`
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

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get the current value of a config key",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configDiffCmd = &cobra.Command{
	Use:   "diff <v1> <v2>",
	Short: "Show what changed between two config versions",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigDiff,
}

var configWatchCmd = &cobra.Command{
	Use:   "watch <key>",
	Short: "Stream live value changes for a config key",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigWatch,
}

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)

	configSetCmd.Flags().StringVar(&configNamespace, "namespace", "default", "namespace")
	configSetCmd.Flags().StringVar(&configTarget, "target", "", "label selector e.g. env=prod,region=us")
	configSetCmd.Flags().StringVar(&configTTL, "ttl", "", "TTL e.g. 24h")
	configSetCmd.Flags().StringVar(&configPushedBy, "pushed-by", "cli", "actor name")
	configCmd.AddCommand(configGetCmd)
	configGetCmd.Flags().StringVar(&configNamespace, "namespace", "default", "namespace")
	configCmd.AddCommand(configDiffCmd)
	configCmd.AddCommand(configWatchCmd)
	configWatchCmd.Flags().StringVar(&configNamespace, "namespace", "default", "namespace")
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
if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(&result); err != nil {
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

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	resp, err := httpClient.Get(getURL(fmt.Sprintf("/config/%s?namespace=%s", key, configNamespace)))
	if err != nil {
		fmt.Printf("%s could not reach control plane: %s\n", crossMark, err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("%s key %s not found in namespace %s\n",
			crossMark,
			cyan.Render(key),
			yellow.Render(configNamespace),
		)
		return nil
	}

	var result configGetResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("%s unexpected response: %s\n", crossMark, string(respBody))
		return err
	}

	valueBytes, _ := json.Marshal(result.Value)
	clockBytes, _ := json.Marshal(result.VectorClock)

	fmt.Printf("\n%s  %s\n", cyan.Render(result.Key), dim.Render(fmt.Sprintf("(%s)", result.Namespace)))
	fmt.Printf("  %s  %s\n", dim.Render("value:  "), bold.Render(string(valueBytes)))
	fmt.Printf("  %s  %s\n", dim.Render("version:"), dim.Render(result.VersionID))
	fmt.Printf("  %s  %s\n", dim.Render("status: "), yellow.Render(result.Status))
	fmt.Printf("  %s  %s\n", dim.Render("by:     "), dim.Render(result.PushedBy))
	fmt.Printf("  %s  %s\n", dim.Render("clock:  "), dim.Render(string(clockBytes)))
	fmt.Println()

	return nil
}

func runConfigDiff(cmd *cobra.Command, args []string) error {
	v1 := args[0]
	v2 := args[1]

	resp, err := httpClient.Get(getURL(fmt.Sprintf("/config/diff?v1=%s&v2=%s", v1, v2)))
	if err != nil {
		fmt.Printf("%s could not reach control plane: %s\n", crossMark, err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("%s %s\n", crossMark, string(respBody))
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("%s unexpected response: %s\n", crossMark, string(respBody))
		return err
	}

	v1Data := result["v1"].(map[string]any)
	v2Data := result["v2"].(map[string]any)

	oldVal, _ := json.Marshal(v1Data["value"])
	newVal, _ := json.Marshal(v2Data["value"])

	fmt.Printf("\n%s  %s\n\n",
		bold.Render("diff"),
		dim.Render(fmt.Sprintf("%s → %s", v1[:8], v2[:8])),
	)
	fmt.Printf("  %s  %s\n", dim.Render("key:      "), cyan.Render(fmt.Sprintf("%v", result["key"])))
	fmt.Printf("  %s  %s\n", dim.Render("namespace:"), yellow.Render(fmt.Sprintf("%v", result["namespace"])))
	fmt.Printf("  %s  %s → %s\n",
		dim.Render("value:    "),
		red.Render(string(oldVal)),
		green.Render(string(newVal)),
	)
	fmt.Printf("  %s  %s\n", dim.Render("pushed by:"), dim.Render(fmt.Sprintf("%v", result["pushed_by"])))
	fmt.Println()

	return nil
}

func runConfigWatch(cmd *cobra.Command, args []string) error {
	key := args[0]

	url := getURL(fmt.Sprintf("/config/%s/watch?namespace=%s", key, configNamespace))

	fmt.Printf("\n%s  %s\n\n",
		bold.Render(fmt.Sprintf("watching %s", cyan.Render(key))),
		dim.Render(fmt.Sprintf("(%s)", configNamespace)),
	)

	resp, err := httpClient.Get(url)
	if err != nil {
		fmt.Printf("%s could not connect: %s\n", crossMark, err)
		return err
	}
	defer resp.Body.Close()

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
		case "watching":
			fmt.Printf("%s connected, waiting for changes...\n", checkMark)
		case "config_push":
			versionID, _ := event["version_id"].(string)
			delta, _ := event["delta"].(map[string]any)
			value := delta[key]
			valueBytes, _ := json.Marshal(value)
			fmt.Printf("%s  %s  version=%s\n",
				green.Render("→"),
				bold.Render(string(valueBytes)),
				dim.Render(versionID[:8]),
			)
		}
	}

	return nil
}