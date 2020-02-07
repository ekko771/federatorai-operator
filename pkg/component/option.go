package component

type ComponentConfigOption func(*ComponentConfig)

func WithNamespace(namespace string) ComponentConfigOption {
	return func(cc *ComponentConfig) {
		cc.NameSpace = namespace
	}
}

func WithImageConfig(ic ImageConfig) ComponentConfigOption {
	return func(cc *ComponentConfig) {
		cc.Image = ic
	}
}

func WithPodSecurityPolicyGroup(podSecurityPolicyGroup string) ComponentConfigOption {
	return func(cc *ComponentConfig) {
		cc.PodSecurityPolicyGroup = podSecurityPolicyGroup
	}
}

func WithPodSecurityPolicyVersion(podSecurityPolicyVersion string) ComponentConfigOption {
	return func(cc *ComponentConfig) {
		cc.PodSecurityPolicyVersion = podSecurityPolicyVersion
	}
}

func WithPrometheusConfig(config PrometheusConfig) ComponentConfigOption {
	return func(cc *ComponentConfig) {
		cc.Prometheus = config
	}
}

func WithKafkaConfig(config KafkaConfig) ComponentConfigOption {
	return func(cc *ComponentConfig) {
		cc.Kafka = config
	}
}
