package commands

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var (
	controlPlaneURL string
	httpClient      = &http.Client{Timeout: 10 * time.Second}
)

var RootCmd = &cobra.Command{
	Use:   "herald",
	Short: "Herald — distributed config propagation engine",
	Long:  `Herald pushes typed config changes to running services with consensus, vector clocks, and one-command rollback.`,
}

func init() {
	RootCmd.PersistentFlags().StringVar(
		&controlPlaneURL,
		"url",
		"http://localhost:7700",
		"control plane URL",
	)
}

func getURL(path string) string {
	return fmt.Sprintf("%s%s", controlPlaneURL, path)
}