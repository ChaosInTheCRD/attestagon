package flags


import (
   "fmt"
   "flag"

   "github.com/go-logr/logr"
   "k8s.io/client-go/rest"
   "k8s.io/cli-runtime/pkg/genericclioptions"
   "github.com/spf13/pflag"
   "k8s.io/klog/v2"
   "k8s.io/klog/v2/klogr"
)

type RegisterFunc func(fs *pflag.FlagSet)

// Flags is a shared struct that stores and manages flags for an app.
type Flags struct {
	logLevel        string
	kubeConfigFlags *genericclioptions.ConfigFlags
	extra           map[string]RegisterFunc

	// RestConfig is the shared based rest config to connect to the Kubernetes
	// API.
	RestConfig *rest.Config

	// DriverName is the driver name as installed in Kubernetes.
	DriverName string

	// Logr is a shared logger.
	Logr logr.Logger
}

func (f *Flags) Complete() error {
	klog.InitFlags(nil)
	f.Logr = klogr.New()
	flag.Set("v", f.logLevel)

	var err error
	f.RestConfig, err = f.kubeConfigFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes rest config: %s", err)
	}

	return nil
}
