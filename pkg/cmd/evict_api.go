package cmd

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

const (
	EvictionSubresource = "pods/eviction"
	EvictionKind        = "Eviction"
)

type Client interface {
	EvictPod(ctx context.Context, pod corev1.Pod) error
}

func evictGroupVersion(clientset kubernetes.Interface) schema.GroupVersion {
	discoveryClient := clientset.Discovery()
	resourceList, err := discoveryClient.ServerResourcesForGroupVersion("v1")
	if err != nil {
		return schema.GroupVersion{}
	}

	for _, resource := range resourceList.APIResources {
		if resource.Name == EvictionSubresource && resource.Kind == EvictionKind && len(resource.Group) > 0 && len(resource.Version) > 0 {
			return schema.GroupVersion{Group: resource.Group, Version: resource.Version}
		}
	}

	return schema.GroupVersion{}
}

func NewEvictClient(clientset kubernetes.Interface) Client {
	switch evictGroupVersion(clientset) {
	case policyv1.SchemeGroupVersion:
		return &ClientV1{clientset}
	default:
		return &ClientV1beta1{clientset}
	}
}

type ClientV1 struct {
	client kubernetes.Interface
}

func (c *ClientV1) EvictPod(ctx context.Context, pod corev1.Pod) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	return c.client.PolicyV1().Evictions(eviction.Namespace).Evict(context.TODO(), eviction)
}

type ClientV1beta1 struct {
	client kubernetes.Interface
}

func (c *ClientV1beta1) EvictPod(ctx context.Context, pod corev1.Pod) error {
	eviction := &policyv1beta1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	return c.client.PolicyV1beta1().Evictions(eviction.Namespace).Evict(context.TODO(), eviction)
}
