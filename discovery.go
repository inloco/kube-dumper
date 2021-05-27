package main

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type discovery interface {
	discover() ([]schema.GroupVersionResource, error)
}

type dumpDiscovery struct {
	clusterConfig *rest.Config
}

func newDiscovery(clusterConfig *rest.Config) discovery {
	return &dumpDiscovery{
		clusterConfig: clusterConfig,
	}
}

func (d *dumpDiscovery) discover() ([]schema.GroupVersionResource, error) {
	clientset, err := kubernetes.NewForConfig(d.clusterConfig)
	if err != nil {
		return nil, err
	}

	lists, err := clientset.DiscoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	return d.parseGvrs(lists)
}

func (d *dumpDiscovery) parseGvrs(lists []*metav1.APIResourceList) ([]schema.GroupVersionResource, error) {
	gvrs := []schema.GroupVersionResource{}
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return nil, err
		}

		for _, apiResource := range list.APIResources {
			if d.hasWatchVerb(apiResource.Verbs) {
				gvrs = append(gvrs, schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: apiResource.Name,
				})
			}
		}
	}
	return gvrs, nil
}

func (d *dumpDiscovery) hasWatchVerb(verbs []string) bool {
	for _, verb := range verbs {
		if verb == "watch" {
			return true
		}
	}
	return false
}
