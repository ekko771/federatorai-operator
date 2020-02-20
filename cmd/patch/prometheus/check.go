package prometheus

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	PromCheckCmd = &cobra.Command{
		Use:   "prom_check",
		Short: "prom_check ",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			InitConfig()
			InitK8SClient()

			ok, _, err := RulesCheck(k8sCli)
			if err == nil && !ok {
				logger.Info("some rules are missing and need to patch")
				os.Exit(1)
			}
			/*
				ok, err = RelabelingCheck(k8sCli)
				if err == nil && !ok {
					logger.Info("some reabelings are missing and need to patch")
					os.Exit(1)
				}
			*/
			return err
		},
	}
)
