package prometheus

import (
	"github.com/spf13/cobra"
)

var configurationFilePath string

var (
	PromApplyCmd = &cobra.Command{
		Use:   "prom_apply",
		Short: "prom_apply ",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			InitConfig()
			InitK8SClient()

			ok, missingRulesMap, err := RulesCheck(k8sCli)
			if err != nil {
				return err
			} else if !ok {
				if err = PatchMissingRules(k8sCli, missingRulesMap); err != nil {
					return err
				}
			}

			/*
				ok, err = RelabelingCheck(k8sCli)
				if err != nil {
					return err
				} else if !ok {
					if err = PatchRelabelings(k8sCli); err != nil {
						return err
					}
				}
			*/
			return nil
		},
	}
)
