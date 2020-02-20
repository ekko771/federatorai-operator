package utils

import (
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetPromConfigFromSecret(k8sCli client.Client, ns, name string) (string, error) {
	secretIns := &corev1.Secret{}
	if err := GetResource(k8sCli, client.ObjectKey{
		Namespace: ns,
		Name:      name,
	}, secretIns); err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(base64.StdEncoding.EncodeToString(secretIns.Data["prometheus.yaml.gz"]))
	if err != nil {
		return "", err
	}

	res, err := GUnZip(decoded)
	if err != nil {
		return "", err
	}
	return string(res), nil
}
