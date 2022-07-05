package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cilium/tetragon/api/v1/tetragon"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"

	//"k8s.io/client-go/pkg/api/v1"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/logs"
)

// PodLoggingController logs the name and namespace of pods that are added,
// deleted, or updated
type PodLoggingController struct {
	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
}

type Config struct {
  Artifacts  []Artifact
}

type Artifact struct {
  Name string
  Ref string
}

type ReturnEvent struct {
        Pod            Pod
        CommandsExecuted []CommandsExecuted
        ProcessExecute map[string]int
        FilesystemMount []FilesystemMount
        TCPConnect []TCPConnection
        UIDSet map[int]int
        FileWrite map[string]int
        FileRead map[string]int
        FileOpen map[string]int
}

type Pod struct {
  Name string
  Namespace string
}

type CommandsExecuted struct {
  Command string
  Arguments string
}

type FilesystemMount struct {
  Source string
  Destination string
}

type TCPConnection struct {
  SocketAddress         string
  SocketPort            int
  DestinationAddress    string
  DestinationPort       int
}

// Run starts shared informers and waits for the shared informer cache to
// synchronize.
func (c *PodLoggingController) Run(stopCh chan struct{}) error {
	// Starts all the shared informers that have been created by the factory so
	// far.
	c.informerFactory.Start(stopCh)
	// wait for the initial synchronization of the local cache.
	if !cache.WaitForCacheSync(stopCh, c.podInformer.Informer().HasSynced) {
		return fmt.Errorf("Failed to sync")
	}
	return nil
}

func (c *PodLoggingController) addFunc(obj interface{}) {
    pod := obj.(*v1.Pod)

    if pod.Status.Phase != "Succeeded" {
        return
    }

    fmt.Println("Processing pod", pod.Name)

    var returnEvent ReturnEvent
    returnEvent.Pod.Name = pod.Name
    returnEvent.Pod.Namespace = pod.Namespace
    returnEvent.FindEvents()

    b, err := json.MarshalIndent(returnEvent, "", "      ")
    if err != nil {
      fmt.Println("error:", err)
    }
    fmt.Printf("Processes executed for pod %s: %s", pod.Name, string(b))
}

func (c *PodLoggingController) updateFunc(oldObj interface{}, newObj interface{}) {
    pod := newObj.(*v1.Pod)

    if pod.Status.Phase != "Succeeded" {
      fmt.Printf("POD %s HAS PHASE %s SO NOT ATTESTING\n", pod.Name, pod.Status.Phase)
      return
    }
    fmt.Println("Processing pod", pod.Name)

    var returnEvent ReturnEvent
    returnEvent.Pod.Name = pod.Name
    returnEvent.Pod.Namespace = pod.Namespace
    returnEvent.FindEvents()

    b, err := json.MarshalIndent(returnEvent, "", "      ")
    if err != nil {
      fmt.Println("error:", err)
    }
    fmt.Printf("Processes executed for pod %s: %s", pod.Name, string(b))
}


// NewPodLoggingController creates a PodLoggingController
func NewPodLoggingController(informerFactory informers.SharedInformerFactory) *PodLoggingController {
	podInformer := informerFactory.Core().V1().Pods()

	c := &PodLoggingController{
		informerFactory: informerFactory,
		podInformer:     podInformer,
	}
	podInformer.Informer().AddEventHandler(
                // Setting handler functions
		cache.ResourceEventHandlerFuncs{
			// Called on creation
			AddFunc: c.addFunc,
			// Called on resource update and every resyncPeriod on existing resources.
			UpdateFunc: c.updateFunc,
		},
	)
	return c
}

var kubeConfigPath, configPath string


func init() {
  home, err := os.UserHomeDir()
  if err != nil {
    panic(err)
  }
  flag.StringVar(&kubeConfigPath, "kubeconfigPath", fmt.Sprintf("%s/.kube/config", home), "absolute path to the kubeconfig file")
  flag.StringVar(&configPath, "configPath", "./config.yaml", "absolute path to the attestagon config file")
}

func main() {
	flag.Parse()
	logs.InitLogs()
	defer logs.FlushLogs()

        // Get Cluster Config
        kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
        if err != nil {
          fmt.Printf("ERROR: Failed to get cluster config: %s", string(err.Error()))
          klog.Fatal(err)
        }

        // Get Attestagon Config
        config, err := loadConfig(configPath)
        if err != nil {
          fmt.Printf("ERROR: Failed to get attestagon config from path %s: %s\n", configPath, string(err.Error()))
        }

        clientset, err := kubernetes.NewForConfig(kubeConfig)
        if err != nil {
          fmt.Printf("ERROR: Failed to get cluster config at path %s: %s\n", kubeConfig, string(err.Error())) 
          klog.Fatal(err)
        }

	factory := informers.NewSharedInformerFactory(clientset, time.Hour*24)
	controller := NewPodLoggingController(factory)
	stop := make(chan struct{})
	defer close(stop)
	err = controller.Run(stop)
	if err != nil {
		klog.Fatal(err)
	}
	select {}
}

func loadConfig(configPath string) (Config, error) {
  c, err := os.ReadFile(configPath)
  if err != nil {
    return Config{}, err
  }

  var config Config
  err = json.Unmarshal(c, &config)
  if err != nil {
    return Config{}, err
  }

  return config, nil
}


func (e *ReturnEvent) FindEvents() error {

    // Get Cluster Config
    path := os.Getenv("HOME") + "/.kube/config"
    config, err := clientcmd.BuildConfigFromFlags("", path)
    if err != nil {
      fmt.Printf("ERROR: Failed to get cluster config: %s", string(err.Error()))
      klog.Fatal(err)
    }

    fmt.Println("Got Cluster Config")

    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
      fmt.Printf("ERROR: Failed to get cluster config: %s", string(err.Error())) 
      klog.Fatal(err)
    }

    listOpts := metav1.ListOptions{
      LabelSelector: "app.kubernetes.io/name=tetragon",
    }

    req, err := clientset.CoreV1().Pods("kube-system").List(context.TODO(), listOpts)
    if err != nil {
      fmt.Printf("ERROR: Failed to get tetragon pod names: %s", string(err.Error()))
      return err
    }

    // Configuring Pod Log Options
    podLogOpts := corev1.PodLogOptions{
      Container: "export-stdout",
      Follow: false,
    }
    
    ctx, cancel := context.WithCancel(context.Background())

    defer cancel()


    wg := sync.WaitGroup{}

    fmt.Printf("Tetragon pods are %s and %s\n", req.Items[0].Name, req.Items[1].Name)

    for i := 0; i < len(req.Items); i++ {
      wait := false
      for wait != true {
        pod, err := clientset.CoreV1().Pods("kube-system").Get(context.TODO(),req.Items[i].Name, metav1.GetOptions{})
        if err != nil {
          panic(err)
        }

        if pod.Status.Phase == "Running" {
          wait = true
        }

      }
      fmt.Printf("Scraping pod %s for events...\n", req.Items[i].Name)
      wg.Add(1)
      go e.PodLogProcessor(ctx, &wg, clientset, req.Items[i].Name, "kube-system", podLogOpts)
    }

    wg.Wait()
    fmt.Println("Successfully Found Events from logs")
    return nil
}


func (e *ReturnEvent) PodLogProcessor(syncCtx context.Context, wg *sync.WaitGroup, clientset *kubernetes.Clientset, podName string, namespace string, podLogOpts corev1.PodLogOptions) {

  defer wg.Done()

  // Getting Logs
  req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
  ctx := context.TODO()
  podLogs, err := req.Stream(ctx)
  if err != nil {
    fmt.Printf("ERROR: Error in opening stream")
  }

  defer podLogs.Close()


  // Setup a Reader for the logs
  r := bufio.NewReader(podLogs)

  // Loop over the reader to find new lines when they arrive
  for {
        select {
          case <-ctx.Done():
            return
          default:
            outputLine, _, err := r.ReadLine()
            if err != nil {
              if err.Error() == "EOF" {
                fmt.Println("Reached end of file for pod name ", podName)
                return
              }
              panic(err)
            } else {
                var response *tetragon.GetEventsResponse 
                json.Unmarshal(outputLine, &response)
                
                e.ProcessEvent(response)
            }
          }
  }
}

func (e *ReturnEvent) ProcessEvent(response *tetragon.GetEventsResponse) error {
  switch response.Event.(type) {
  case *tetragon.GetEventsResponse_ProcessExec:
          exec := response.GetProcessExec()
          if exec.Process == nil {
            return fmt.Errorf("process field is not set")
          }

          if exec.Process.Pod.Name == e.Pod.Name && exec.Process.Pod.Namespace == e.Pod.Namespace {
            if e.ProcessExecute == nil {
              e.ProcessExecute = make(map[string]int)
            }

            e.ProcessExecute[exec.Process.Binary] = e.ProcessExecute[exec.Process.Binary] + 1

            // Adding command execution to the "CommandsExecuted"
            if e.CommandsExecuted == nil {
              e.CommandsExecuted = make([]CommandsExecuted, 0)
            }

            e.CommandsExecuted = append(e.CommandsExecuted, CommandsExecuted{Command: exec.Process.Binary, Arguments: exec.Process.Arguments})
          }

          return nil
  case *tetragon.GetEventsResponse_ProcessExit:
          return nil
  case *tetragon.GetEventsResponse_ProcessKprobe:
          kprobe := response.GetProcessKprobe()
          if kprobe.Process == nil {
            return fmt.Errorf("process field is not set")
          }
          if kprobe.Process.Pod.Name == e.Pod.Name && kprobe.Process.Pod.Namespace == e.Pod.Namespace {
            switch kprobe.FunctionName {
            case "__x64_sys_write":
              // Check that there is a file argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
                    if e.FileWrite == nil {
                      e.FileWrite = make(map[string]int)
                    }

                    e.FileWrite[kprobe.Args[0].GetFileArg().Path] = e.FileWrite[kprobe.Args[0].GetFileArg().Path] + 1
              }
              return nil
            case "__x64_sys_read":
              // Check that there is a file argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
                    if e.FileRead == nil {
                      e.FileRead = make(map[string]int)
                    }

                    e.FileRead[kprobe.Args[0].GetFileArg().Path] = e.FileRead[kprobe.Args[0].GetFileArg().Path] + 1
              }
              return nil
            case "fd_install":
              // Check that there is a file argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[1] != nil && kprobe.Args[1].GetFileArg() != nil {
                    if e.FileOpen == nil {
                      e.FileOpen = make(map[string]int)
                    }

                    e.FileOpen[kprobe.Args[1].GetFileArg().Path] = e.FileOpen[kprobe.Args[1].GetFileArg().Path] + 1
                }
              return nil
            case "__x64_sys_mount":
              // Check that there is an argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[1] != nil {
                    if e.FilesystemMount == nil {
                      e.FilesystemMount = make([]FilesystemMount, 0)
                    }

                    e.FilesystemMount = append(e.FilesystemMount, FilesystemMount{Source: kprobe.Args[0].GetStringArg(), Destination: kprobe.Args[1].GetStringArg()})
                }
              return nil
            case "__x64_sys_setuid":
              // Check that there is an argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
                    if e.UIDSet == nil {
                      e.UIDSet = make(map[int]int)
                    }

                    e.UIDSet[int(kprobe.Args[0].GetIntArg())] = e.UIDSet[int(kprobe.Args[0].GetIntArg())] + 1 
                }
              return nil
            case "tcp_connect":
              // Check that there is an argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
                    if e.TCPConnect == nil {
                      e.TCPConnect = make([]TCPConnection, 0)
                    }
                    
                    sa := kprobe.Args[0].GetSockArg()
                    e.TCPConnect = append(e.TCPConnect, TCPConnection{SocketAddress: sa.Saddr, SocketPort: int(sa.Sport), DestinationAddress: sa.Daddr, DestinationPort: int(sa.Dport)})
                }
              return nil
            default:
                    return nil
            }
          }
          return nil
  case *tetragon.GetEventsResponse_ProcessDns:
          // dns := response.GetProcessDns()
          return nil
  case *tetragon.GetEventsResponse_ProcessTracepoint:
          // tp := response.GetProcessTracepoint()
          return nil
  }

  return fmt.Errorf("unknown event type")
}
