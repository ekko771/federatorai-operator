package prometheus

import (
	"errors"
	"strings"

	"github.com/containers-ai/federatorai-operator/pkg/consts"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = log.Log.WithName("patch-prometheus")

func InitConfig() {
	viper.SetEnvPrefix(consts.EnvVarPrefix)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(strings.Split(consts.EnvReplacerOldNew, ";")...))
	viper.AllowEmptyEnv(consts.AllowEmptyEnv)
	viper.SetConfigFile(configurationFilePath)

	if err := viper.ReadInConfig(); err != nil {
		panic(errors.New("Read configuration file failed: " + err.Error()))
	}
}

func init() {
	PromCheckCmd.Flags().StringVar(&configurationFilePath, "config", consts.DefaultConfigPath, "File path to federatorai-operator coniguration")
	PromApplyCmd.Flags().StringVar(&configurationFilePath, "config", consts.DefaultConfigPath, "File path to federatorai-operator coniguration")
}
