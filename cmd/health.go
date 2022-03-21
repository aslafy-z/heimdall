package cmd

import (
	"fmt"

	"github.com/dadrus/heimdall/cmd/health"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Commands for checking the status of an ORY Oathkeeper deployment",
	Long: `Note:
  The endpoint URL should point to a single ORY Oathkeeper deployment.
  If the endpoint URL points to a Load Balancer, these commands will effective test the Load Balancer.
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(cmd.UsageString())
	},
}

func init() {
	RootCmd.AddCommand(healthCmd)

	healthCmd.PersistentFlags().StringP("endpoint", "e", "", `The endpoint URL of ORY Oathkeeper's management API. 
Note: The endpoint URL should point to a single ORY Oathkeeper deployment. 
If the endpoint URL points to a Load Balancer, these commands will effective test the Load Balancer.`)
	healthCmd.AddCommand(health.NewAliveCommand())
	healthCmd.AddCommand(health.NewReadyCommand())
}
