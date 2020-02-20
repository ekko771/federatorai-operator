package prometheus

import (
	"errors"

	k8SUtils "github.com/containers-ai/alameda/pkg/utils/kubernetes"
	federatoraiapi "github.com/containers-ai/federatorai-operator/pkg/apis"
	prom_op_api "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var k8sCli client.Client

func InitK8SClient() {
	// Instance kubernetes client
	k8sClient, err := k8SUtils.NewK8SClient()
	if err != nil {
		panic(errors.New("Get kubernetes client failed: " + err.Error()))
	} else {
		k8sCli = k8sClient
		k8sClientConfig, err := config.GetConfig()
		if err != nil {
			panic(errors.New("Failed to get kubernetes configuration: " + err.Error()))
		}

		mgr, err := manager.New(k8sClientConfig, manager.Options{})
		if err != nil {
			panic(err.Error())
		}
		if err := prom_op_api.AddToScheme(mgr.GetScheme()); err != nil {
			panic(err.Error())
		}
		if err := federatoraiapi.AddToScheme(mgr.GetScheme()); err != nil {
			panic(err.Error())
		}
		if err := apiextensionsv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
			panic(err.Error())
		}
	}
}

func GetK8SClient() client.Client {
	return k8sCli
}
