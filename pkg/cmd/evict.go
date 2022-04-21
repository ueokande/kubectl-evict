package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	evictUsageStr = "evict (POD | TYPE/NAME)"

	evictExample = `
	# Evict a pod nginx
	kubectl evict nginx

	# Evict all pods defined by label app=nginx
	kubectl evict -l app=nginx

	# Evict all pods from of a deployment named nginx
	kubectl evict deployment/nginx -c nginx-1

	# Evict all pods from node worker-1
	kubectl evict node/worker-1`
)

var (
	evictUsageErrStr = fmt.Sprintf("expected '%s'.\nPOD or TYPE/NAME is a required argument for the evict command", evictUsageStr)
)

type EvictOptions struct {
	configFlags *genericclioptions.ConfigFlags

	GracePeriodSeconds int64
	DryRun             bool

	ResourceArg string
	Selector    string
	Object      runtime.Object

	genericclioptions.IOStreams
}

func NewEvictOptions(streams genericclioptions.IOStreams) *EvictOptions {
	configFlags := genericclioptions.NewConfigFlags(true)

	return &EvictOptions{
		configFlags: configFlags,

		IOStreams: streams,
	}
}

// NewCmdEvict provides a cobra command wrapping EvictOptions
func NewCmdEvict(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewEvictOptions(streams)

	cmd := &cobra.Command{
		Use:          evictUsageStr,
		Short:        "Evict a pod or specified resource from the cluster",
		Example:      evictExample,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.RunEvict(c.Context()); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&o.Selector, "selector", "l", o.Selector, "Selector (label query) to filter on.")
	cmd.Flags().BoolVar(&o.DryRun, "dry-run", false, "If true, submit server-side request without persisting the resource.")
	cmd.Flags().Int64Var(&o.GracePeriodSeconds, "grace-period", -1, "Period of time in seconds given to the resource to terminate gracefully. Ignored if negative.")

	o.configFlags.AddFlags(cmd.PersistentFlags())

	return cmd
}

func (o *EvictOptions) Complete(cmd *cobra.Command, args []string) error {
	switch len(args) {
	case 0:
		if len(o.Selector) == 0 {
			return fmt.Errorf(evictUsageErrStr)
		}
	case 1:
		o.ResourceArg = args[0]
		if len(o.Selector) != 0 {
			return fmt.Errorf("only a selector (-l) or a resource name is allowed")
		}
	default:
		return fmt.Errorf(evictUsageErrStr)
	}

	var err error
	namespace, namespaceOverwride, err := o.configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}
	b := resource.NewBuilder(o.configFlags).
		WithScheme(scheme.Scheme, scheme.Scheme.PrioritizedVersionsAllGroups()...).
		NamespaceParam(namespace).DefaultNamespace().
		SingleResourceType()
	if o.ResourceArg != "" {
		b.ResourceNames("pods", o.ResourceArg)
	}
	if o.Selector != "" {
		b.ResourceTypes("pods").LabelSelectorParam(o.Selector)
	}

	infos, err := b.Do().Infos()
	if err != nil {
		return err
	}
	if o.Selector == "" && len(infos) != 1 {
		return errors.New("expected a resource")
	}
	o.Object = infos[0].Object
	if o.Selector != "" && len(o.Object.(*corev1.PodList).Items) == 0 {
		return fmt.Errorf("no resources found in %s namespace", namespace)
	}
	if _, ok := o.Object.(*corev1.Node); namespaceOverwride && ok {
		return errors.New("--namespace should not be specified with node target")
	}

	return nil
}

// Run evicts a pod or pods on the resources
func (o *EvictOptions) RunEvict(ctx context.Context) error {
	clientConfig, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}
	api, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return err
	}
	pods, err := podsForObject(ctx, api.CoreV1(), o.Object)
	if err != nil {
		return err
	}

	opts := new(metav1.DeleteOptions)
	if o.GracePeriodSeconds >= 0 {
		opts.GracePeriodSeconds = &o.GracePeriodSeconds
	}
	if o.DryRun {
		opts.DryRun = []string{metav1.DryRunAll}
	}

	verb := "evicted"
	if o.DryRun {
		verb = "evicted (dry-run)"
	}

	eviction := NewEvictClient(api)
	for _, pod := range pods {
		err := eviction.EvictPod(ctx, pod, opts)
		if err != nil {
			return err
		}
		fmt.Fprintf(o.Out, "pod %s/%s %s\n", pod.Namespace, pod.Name, verb)
	}
	return nil
}

func podsForObject(ctx context.Context, api corev1client.CoreV1Interface, object runtime.Object) ([]corev1.Pod, error) {
	switch t := object.(type) {
	case *corev1.PodList:
		return t.Items, nil

	case *corev1.Pod:
		return []corev1.Pod{*t}, nil
	}

	namespace, labelsel, fieldsel, err := selectorsForObject(object)
	if err != nil {
		return nil, fmt.Errorf("cannot get the logs from %T: %v", object, err)
	}

	opt := metav1.ListOptions{}
	if labelsel != nil {
		opt.LabelSelector = labelsel.String()
	}
	if fieldsel != nil {
		opt.FieldSelector = fieldsel.String()
	}

	info, err := api.Pods(namespace).List(ctx, opt)
	if err != nil {
		return nil, err
	}
	return info.Items, nil
}

func selectorsForObject(object runtime.Object) (namespace string, labelsel labels.Selector, fieldsel fields.Selector, err error) {
	switch t := object.(type) {
	case *appsv1.ReplicaSet:
		namespace = t.Namespace
		labelsel, err = metav1.LabelSelectorAsSelector(t.Spec.Selector)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid label selector: %v", err)
		}
	case *corev1.ReplicationController:
		namespace = t.Namespace
		labelsel = labels.SelectorFromSet(t.Spec.Selector)
	case *appsv1.StatefulSet:
		namespace = t.Namespace
		labelsel, err = metav1.LabelSelectorAsSelector(t.Spec.Selector)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid label selector: %v", err)
		}
	case *appsv1.DaemonSet:
		namespace = t.Namespace
		labelsel, err = metav1.LabelSelectorAsSelector(t.Spec.Selector)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid label selector: %v", err)
		}

	case *appsv1.Deployment:
		namespace = t.Namespace
		labelsel, err = metav1.LabelSelectorAsSelector(t.Spec.Selector)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid label selector: %v", err)
		}
	case *batchv1.Job:
		namespace = t.Namespace
		labelsel, err = metav1.LabelSelectorAsSelector(t.Spec.Selector)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid label selector: %v", err)
		}
	case *corev1.Service:
		namespace = t.Namespace
		if t.Spec.Selector == nil || len(t.Spec.Selector) == 0 {
			return "", nil, nil, fmt.Errorf("invalid service '%s': Service is defined without a selector", t.Name)
		}
		labelsel = labels.SelectorFromSet(t.Spec.Selector)
	case *corev1.Node:
		namespace = metav1.NamespaceAll
		fieldsel = fields.SelectorFromSet(fields.Set{"spec.nodeName": t.Name})
	default:
		return "", nil, nil, fmt.Errorf("selector for %T not implemented", object)
	}
	return namespace, labelsel, fieldsel, nil
}
