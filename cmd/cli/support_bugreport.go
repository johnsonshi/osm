package main

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/openservicemesh/osm/pkg/bugreport"
	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/k8s"
)

const bugReportDescription = `
Generate a bug report.

# Specifying the archive format:

If '--out-file' or '-o' is not specified, the bug report will be generated as a
compressed tar file in the tar.gz format. To generate the bug report using
a different archive format, specify the output file along with its extension
type.

The format of the archive is determined by its
file extension. Supported extensions:
  .zip
  .tar
  .tar.gz
  .tgz
  .tar.bz2
  .tbz2
  .tar.xz
  .txz
  .tar.lz4
  .tlz4
  .tar.sz
  .tsz
  .rar (open only)
  .bz2
  .gz
  .lz4
  .sz
  .xz

*Note:
- Both 'osm' and 'kubectl' CLI must reside in the evironment's lookup path.
- If the environment includes sensitive information that should not be collected,
  please do not specify the associated resources. Before sharing the bug report,
  please audit and redact any sensitive information that should not be shared.
`

const bugReportExample = `
# Generate a bug report for the given namespaces, deployments, and pods
osm support bug-report --app-namespaces bookbuyer,bookstore \
	--app-deployments bookbuyer/bookbuyer,bookstore/bookstore-v1 \
	--app-pods bookthief/bookthief-7bb7f9b98c-qplq4
`

type bugReportCmd struct {
	stdout         io.Writer
	stderr         io.Writer
	kubeClient     kubernetes.Interface
	all            bool
	appNamespaces  []string
	appDeployments []string
	appPods        []string
	outFile        string
}

func newSupportBugReportCmd(config *action.Configuration, stdout io.Writer, stderr io.Writer) *cobra.Command {
	bugReportCmd := &bugReportCmd{
		stdout: stdout,
		stderr: stderr,
	}

	cmd := &cobra.Command{
		Use:   "bug-report",
		Short: "generate bug report",
		Long:  bugReportDescription,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			config, err := settings.RESTClientGetter().ToRESTConfig()
			if err != nil {
				return errors.Errorf("Error fetching kubeconfig: %s", err)
			}
			bugReportCmd.kubeClient, err = kubernetes.NewForConfig(config)
			if err != nil {
				return errors.Errorf("Could not access Kubernetes cluster, check kubeconfig: %s", err)
			}
			return bugReportCmd.run()
		},
		Example: bugReportExample,
	}

	f := cmd.Flags()
	f.BoolVar(&bugReportCmd.all, "all", false, "All pods in the mesh")
	f.StringSliceVar(&bugReportCmd.appNamespaces, "app-namespaces", nil, "Application namespaces")
	f.StringSliceVar(&bugReportCmd.appDeployments, "app-deployments", nil, "Application deployments: <namespace>/<deployment>")
	f.StringSliceVar(&bugReportCmd.appPods, "app-pods", nil, "Application pods: <namespace>/<pod>")
	f.StringVarP(&bugReportCmd.outFile, "out-file", "o", "", "Output file with archive format extension")

	return cmd
}

func (cmd *bugReportCmd) run() error {
	var appPods, appDeployments []types.NamespacedName

	if cmd.all {
		ctx := context.Background()
		cmd.appNamespaces = nil
		namespaces, err := cmd.kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
			LabelSelector: constants.OSMKubeResourceMonitorAnnotation,
		})
		if err != nil {
			fmt.Fprintf(cmd.stderr, "Unable to list mesh namespaces")
		}
		for _, namespace := range namespaces.Items {
			namespaceName := namespace.ObjectMeta.Name
			cmd.appNamespaces = append(cmd.appNamespaces, namespaceName)
			pods, err := cmd.kubeClient.CoreV1().Pods(namespaceName).List(ctx, metav1.ListOptions{})
			if err != nil {
				fmt.Fprintf(cmd.stderr, "Unable to get pods from namespace %s", namespaceName)
			}
			for _, pod := range pods.Items {
				nsName := types.NamespacedName{
					Namespace: pod.Namespace,
					Name:      pod.Name,
				}
				appPods = append(appPods, nsName)
			}
		}
	} else {
		for _, pod := range cmd.appPods {
			p, err := k8s.NamespacedNameFrom(pod)
			if err != nil {
				fmt.Fprintf(cmd.stderr, "Pod name %s is not namespaced, skipping it", pod)
				continue
			}
			appPods = append(appPods, p)
		}

		for _, deployment := range cmd.appDeployments {
			d, err := k8s.NamespacedNameFrom(deployment)
			if err != nil {
				fmt.Fprintf(cmd.stderr, "Deployment name %s is not namespaced, skipping it", deployment)
				continue
			}
			appDeployments = append(appDeployments, d)
		}
	}

	bugReportCfg := &bugreport.Config{
		Stdout:               cmd.stdout,
		Stderr:               cmd.stderr,
		KubeClient:           cmd.kubeClient,
		ControlPlaneNamepace: settings.Namespace(),
		AppNamespaces:        cmd.appNamespaces,
		AppDeployments:       appDeployments,
		AppPods:              appPods,
		OutFile:              cmd.outFile,
	}

	return bugReportCfg.Run()
}
