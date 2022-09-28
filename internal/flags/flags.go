package flags


import (
   "flag"
   "fmt"
   "github.com/go-logr/logr"
   "k8s.io/client-go/rest"
   "k8s.io/cli-runtime/pkg/genericclioptions"
   "github.com/spf13/cobra"
   "github.com/spf13/pflag"
   cliflag "k8s.io/component-base/cli/flag"
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

	// Logr is a shared logger.
	Logr logr.Logger
}

// func (f *Flags) Complete() error {
// 	klog.InitFlags(nil)
// 	f.Logr = klogr.New()
// 	flag.Set("v", f.logLevel)
//
// 	var err error
// 	f.RestConfig, err = f.kubeConfigFlags.ToRESTConfig()
// 	if err != nil {
// 		return fmt.Errorf("failed to build kubernetes rest config: %s", err)
// 	}
//
// 	return nil
// }

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


func New() *Flags {
	return &Flags{
		extra: make(map[string]RegisterFunc),
	}
}


func (f *Flags) Add(name string, regFn RegisterFunc) *Flags {
	f.extra[name] = regFn
	return f
}

func (f *Flags) Prepare(cmd *cobra.Command) *Flags {
	f.addFlags(cmd)
	return f
}

func (f *Flags) addFlags(cmd *cobra.Command) {
	var nfs cliflag.NamedFlagSets

	f.addAppFlags(nfs.FlagSet("App"))
	for name, regFn := range f.extra {
		regFn(nfs.FlagSet(name))
	}
	f.kubeConfigFlags = genericclioptions.NewConfigFlags(true)
	f.kubeConfigFlags.AddFlags(nfs.FlagSet("Kubernetes"))

	usageFmt := "Usage:\n  %s\n"
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), nfs, 0)
		return nil
	})

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), nfs, 0)
	})

	fs := cmd.Flags()
	for _, f := range nfs.FlagSets {
		fs.AddFlagSet(f)
	}
}

func (f *Flags) addAppFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&f.logLevel,
		"log-level", "v", "1",
		"Log level (1-5).")
}
