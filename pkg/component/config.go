package component

type InfluxDBConfig struct {
	Address   string
	BasicAuth BasicAuth
}

type PrometheusConfig struct {
	Address         string
	Host            string
	Port            string
	Protocol        string
	BasicAuth       BasicAuth
	BearerTokenFile string
	TLS             TLSConfig
}

type KafkaConfig struct {
	Enabled         bool
	BrokerAddresses []string
	Version         string

	SASL SASLConfig
	TLS  TLSConfig
}

type BasicAuth struct {
	Username string
	Password string
}

type SASLConfig struct {
	Enabled   bool
	BasicAuth BasicAuth
}

type TLSConfig struct {
	Enabled            bool
	InsecureSkipVerify bool
}

type FederatoraiAgentGPUDatasourceConfig struct {
	InfluxDB   InfluxDBConfig
	Prometheus PrometheusConfig
}

type FederatoraiAgentGPUConfig struct {
	Enabled    bool
	Datasource FederatoraiAgentGPUDatasourceConfig
}

func NewDefaultFederatoraiAgentGPUConfig() FederatoraiAgentGPUConfig {
	return FederatoraiAgentGPUConfig{
		Enabled: false,
		Datasource: FederatoraiAgentGPUDatasourceConfig{
			InfluxDB: InfluxDBConfig{
				Address: "",
				BasicAuth: BasicAuth{
					Username: "",
					Password: "",
				},
			},
			Prometheus: PrometheusConfig{
				Address: "",
				BasicAuth: BasicAuth{
					Username: "",
					Password: "",
				},
			},
		},
	}
}

type AIDispatcherConfig struct {
	Enabled bool
}

func NewDefaultAIDispatcherConfig() AIDispatcherConfig {
	return AIDispatcherConfig{
		Enabled: true,
	}
}

type ExecutionConfig struct {
	EnabledVPA bool
}

func NewDefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		EnabledVPA: true,
	}
}
