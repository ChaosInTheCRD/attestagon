package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/in-toto/in-toto-golang/in_toto"
	v02 "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"
	"github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/cmd/cosign/cli/sign"
	"github.com/sigstore/cosign/pkg/cosign"
	cremote "github.com/sigstore/cosign/pkg/cosign/remote"
	"github.com/sigstore/cosign/pkg/oci/mutate"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	"github.com/sigstore/cosign/pkg/oci/static"
	"github.com/sigstore/cosign/pkg/types"
	"github.com/sigstore/sigstore/pkg/signature/dsse"
	signatureoptions "github.com/sigstore/sigstore/pkg/signature/options"

	"github.com/cilium/tetragon/api/v1/tetragon"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	// godigest "github.com/opencontainers/go-digest"

	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"

	//"k8s.io/client-go/pkg/api/v1"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/logs"
)

type ProvenanceStatement struct {
  StatementHeader StatementHeader `json:"statementHeader"`
  Predicate Predicate `json:"predicate"`
}

type StatementHeader struct {
  Type string `json:"type"`
  PredicateType string `json:"predicateType"`
  Subject Subject `json:"subject"`
}

type Subject struct {
  Name string `json:"name"`
  Digest string `json:"digest"`
}

type TerminationMessage struct {
  Key string `json:"key"`
  Value string `json:"value"`
}

// PodLoggingController logs the name and namespace of pods that are added,
// deleted, or updated
type PodLoggingController struct {
	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
}

type Config struct {
  Artifacts  []Artifact `yaml:"artifacts"`
}

type Artifact struct {
  Name string `yaml:"name"`
  Ref string `yaml:"ref"`
}

type Predicate struct {
  Pod            Pod `json:"pod"`
  CommandsExecuted []CommandsExecuted `json:"commandsExecuted"`
  ProcessesExecuted map[string]int `json:"processesExecuted"`
  FilesystemsMounted []FilesystemMounted `json:"fileSystemsMounted"`
  TCPConnections []TCPConnection `json:"tcpConnections"`
  UIDSet map[int]int `json:"uidSet"`
  FilesWritten map[string]int `json:"filesWritten"`
  FilesRead map[string]int `json:"filesRead"`
  FilesOpened map[string]int `json:"filesOpened"`
}

type Pod struct {
  Name string
  Namespace string
}

type CommandsExecuted struct {
  Command string
  Arguments string
}

type FilesystemMounted struct {
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

    err := processPod(pod)
    if err != nil{
      fmt.Println("error:", err)
    }
}

func (c *PodLoggingController) updateFunc(oldObj interface{}, newObj interface{}) {

  pod := newObj.(*v1.Pod)
  
  err := processPod(pod)
  if err != nil{
    fmt.Println("error:", err)
  }
}

func processPod(pod *v1.Pod) error {

  // Get Attestagon Config
  config, err := loadConfig(configPath)
  if err != nil {
    err = fmt.Errorf("ERROR: Failed to get attestagon config from path %s: %s\n", configPath, string(err.Error())) 
    return err 
  }

  for i:= 0; i < len(config.Artifacts); i++ {
    if pod.Annotations["attestagon.io/artifact"] == config.Artifacts[i].Name && config.Artifacts[i].Name != "" && pod.Status.Phase == "Succeeded" && pod.Annotations["attestagon.io/attested"] != "true" {
        fmt.Println("Processing pod", pod.Name)

        var predicate Predicate
        predicate.Pod.Name = pod.Name
        predicate.Pod.Namespace = pod.Namespace
        predicate.FindEvents()

        statement := in_toto.Statement{
          StatementHeader: in_toto.StatementHeader{
            Type: "https://in-toto.io/Statement/v0.1",
            PredicateType: "https://attestagon.io/provenance/v0.1",
            Subject: []in_toto.Subject{{Name: config.Artifacts[i].Name}},
          },
          Predicate: predicate,
        }
        stat, _ := json.Marshal(statement)

        fmt.Printf("%s\n", string(stat))

        for _, status := range pod.Status.ContainerStatuses {
          message := []TerminationMessage{}
          json.Unmarshal([]byte(status.State.Terminated.Message), &message)

          for i := 0; i < len(message); i++ {
            if message[i].Key == "digest" {
              // _, err := godigest.Parse(message[i].Key)
              // if err != nil {
                // return fmt.Errorf("Digest (%s) found in termination message for container %s in pod %s not valid digest:", message[i].Value, status.Name, pod.Name)
              // }

              fmt.Printf("Ready to sign and push the attestation!\n")
              ctx := context.TODO()
              imageRef := fmt.Sprintf("%s@%s", config.Artifacts[i].Ref, message[i].Value)
              err := SignAndPush(ctx, statement, imageRef)
              if err != nil {
               fmt.Printf("ERROR: Error signing and pushing: %s", string(err.Error()))
              }

              fmt.Println("And I think that's it! Marking the pod as attested.")


              // TODO: Dont define another client!
              kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
              if err != nil {
                fmt.Printf("ERROR: Failed to get cluster config: %s", string(err.Error()))
                klog.Fatal(err)
              }

              clientset, err := kubernetes.NewForConfig(kubeConfig)
              if err != nil {
                fmt.Printf("ERROR: Failed to get cluster config at path %s: %s\n", kubeConfig, string(err.Error())) 
                klog.Fatal(err)
              }

              patch := []byte(`{"metadata":{"annotations":{"attestagon.io/attested": "true"}}}`)
              pod, err := clientset.CoreV1().Pods(pod.Namespace).Patch(context.TODO(), pod.Name, ktypes.StrategicMergePatchType, patch, metav1.PatchOptions{})
              if err != nil {
                panic(err)
              }

              if pod.ObjectMeta.Annotations["attestagon.io/attested"] != "true" {
                panic("Something is clearly wrong.")
              }

            }
          }
        }
    }
  }

  fmt.Printf("Skipping pod %s in namespace %s\n", pod.Name, pod.Namespace)
  return nil
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
  flag.StringVar(&kubeConfigPath, "kubeconfigPath", "", "absolute path to the kubeconfig file")
  flag.StringVar(&configPath, "configPath", os.Getenv("CONFIG_PATH"), "absolute path to the attestagon config file")
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
  err = yaml.Unmarshal(c, &config)
  if err != nil {
    return Config{}, err
  }

  return config, nil
}


func (e *Predicate) FindEvents() error {

  // Get Cluster Config
  // THIS IS WOEFUL AND WILL NOT WORK IN LOCAL
  config, err := clientcmd.BuildConfigFromFlags("", "")
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


func (p *Predicate) PodLogProcessor(syncCtx context.Context, wg *sync.WaitGroup, clientset *kubernetes.Clientset, podName string, namespace string, podLogOpts corev1.PodLogOptions) {

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
                
                p.ProcessEvent(response)
            }
          }
  }
}

func (p *Predicate) ProcessEvent(response *tetragon.GetEventsResponse) error {
  switch response.Event.(type) {
  case *tetragon.GetEventsResponse_ProcessExec:
          exec := response.GetProcessExec()
          if exec.Process == nil {
            return fmt.Errorf("process field is not set")
          }

          if exec.Process.Pod.Name == p.Pod.Name && exec.Process.Pod.Namespace == p.Pod.Namespace {
            if p.ProcessesExecuted == nil {
              p.ProcessesExecuted = make(map[string]int)
            }

            p.ProcessesExecuted[exec.Process.Binary] = p.ProcessesExecuted[exec.Process.Binary] + 1

            // Adding command execution to the "CommandsExecuted"
            if p.CommandsExecuted == nil {
              p.CommandsExecuted = make([]CommandsExecuted, 0)
            }

            p.CommandsExecuted = append(p.CommandsExecuted, CommandsExecuted{Command: exec.Process.Binary, Arguments: exec.Process.Arguments})
          }

          return nil
  case *tetragon.GetEventsResponse_ProcessExit:
          return nil
  case *tetragon.GetEventsResponse_ProcessKprobe:
          kprobe := response.GetProcessKprobe()
          if kprobe.Process == nil {
            return fmt.Errorf("process field is not set")
          }
          if kprobe.Process.Pod.Name == p.Pod.Name && kprobe.Process.Pod.Namespace == p.Pod.Namespace {
            switch kprobe.FunctionName {
            case "__x64_sys_write":
              // Check that there is a file argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
                    if p.FilesWritten == nil {
                      p.FilesWritten = make(map[string]int)
                    }

                    p.FilesWritten[kprobe.Args[0].GetFileArg().Path] = p.FilesWritten[kprobe.Args[0].GetFileArg().Path] + 1
              }
              return nil
            case "__x64_sys_read":
              // Check that there is a file argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
                    if p.FilesRead == nil {
                      p.FilesRead = make(map[string]int)
                    }

                    p.FilesRead[kprobe.Args[0].GetFileArg().Path] = p.FilesRead[kprobe.Args[0].GetFileArg().Path] + 1
              }
              return nil
            case "fd_install":
              // Check that there is a file argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[1] != nil && kprobe.Args[1].GetFileArg() != nil {
                    if p.FilesOpened == nil {
                      p.FilesOpened = make(map[string]int)
                    }

                    p.FilesOpened[kprobe.Args[1].GetFileArg().Path] = p.FilesOpened[kprobe.Args[1].GetFileArg().Path] + 1
                }
              return nil
            case "__x64_sys_mount":
              // Check that there is an argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[1] != nil {
                    if p.FilesystemsMounted == nil {
                      p.FilesystemsMounted = make([]FilesystemMounted, 0)
                    }

                    p.FilesystemsMounted = append(p.FilesystemsMounted, FilesystemMounted{Source: kprobe.Args[0].GetStringArg(), Destination: kprobe.Args[1].GetStringArg()})
                }
              return nil
            case "__x64_sys_setuid":
              // Check that there is an argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
                    if p.UIDSet == nil {
                      p.UIDSet = make(map[int]int)
                    }

                    p.UIDSet[int(kprobe.Args[0].GetIntArg())] = p.UIDSet[int(kprobe.Args[0].GetIntArg())] + 1 
                }
              return nil
            case "tcp_connect":
              // Check that there is an argument to log
              if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
                    if p.TCPConnections == nil {
                      p.TCPConnections = make([]TCPConnection, 0)
                    }
                    
                    sa := kprobe.Args[0].GetSockArg()
                    p.TCPConnections = append(p.TCPConnections, TCPConnection{SocketAddress: sa.Saddr, SocketPort: int(sa.Sport), DestinationAddress: sa.Daddr, DestinationPort: int(sa.Dport)})
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


// The implementation of signing the attestation is not secure, and is for testing purposes only
var keyPass = []byte("hello")

var passFunc = func(_ bool) ([]byte, error) {
	return keyPass, nil
}

func keypair() (*cosign.KeysBytes, string, string, error) {

	keys, err := cosign.GenerateKeyPair(passFunc)
	if err != nil {
	  return nil, "", "", err		
        }

	privKeyPath := os.Getenv("COSIGN_KEY")
        fmt.Printf("COSIGN KEY PATH IS: %s \n", privKeyPath)

	pubKeyPath := os.Getenv("COSIGN_PUB")
        fmt.Printf("COSIGN KEY PATH IS: %s \n", pubKeyPath)

	return keys, privKeyPath, pubKeyPath, nil
}

func SignAndPush(ctx context.Context, statement in_toto.Statement, imageRef string) error {

        ref, err := name.ParseReference(imageRef)
        if err != nil {
                return fmt.Errorf("parsing reference: %w", err)
        }

        regOpts := options.RegistryOptions{}

        ociremoteOpts, err := regOpts.ClientOpts(ctx)
	if err != nil {
		return err
	}
	digest, err := ociremote.ResolveDigest(ref, ociremoteOpts...)
	if err != nil {
		return err
	}

        // TODO - Assess whether it gives any more validation that the hash and reference matches up from adding it to the subject here.
	h, _ := gcrv1.NewHash(digest.Identifier())

        statement.StatementHeader.Subject[0].Digest = v02.DigestSet{"sha256":h.Hex}
	// Overwrite "ref" with a digest to avoid a race where we use a tag
	// multiple times, and it potentially points to different things at
	// each access.
	ref = digest // nolint


        ko := options.KeyOpts{KeyRef: "cosign.key", PassFunc: passFunc}
        
  	sv, err := sign.SignerFromKeyOpts(ctx, "", "", ko)
	if err != nil {
		return fmt.Errorf("getting signer: %w", err)
	}
        defer sv.Close()

        wrapped := dsse.WrapSigner(sv, types.IntotoPayloadType)
	dd := cremote.NewDupeDetector(sv)

	payload, err := json.Marshal(statement)
	if err != nil {
		return err
	}
	signedPayload, err := wrapped.SignMessage(bytes.NewReader(payload), signatureoptions.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("signing: %w", err)
	}

	opts := []static.Option{static.WithLayerMediaType(types.DssePayloadType)}



	sig, err := static.NewAttestation(signedPayload, opts...)
	if err != nil {
		return err
	}

	se, err := ociremote.SignedEntity(digest, ociremoteOpts...)
	if err != nil {
		return err
	}

	signOpts := []mutate.SignOption{
		mutate.WithDupeDetector(dd),
	}

	// Attach the attestation to the entity.
	newSE, err := mutate.AttachAttestationToEntity(se, sig, signOpts...)
	if err != nil {
		return err
	}

	// Publish the attestations associated with this entity
        err = ociremote.WriteAttestations(digest.Repository, newSE, ociremoteOpts...)
        if err != nil {
           return err
        }

        return nil
}

func contains(s []int, e int) bool {
    for _, a := range s {
        if a == e {
            return true
        }
    }
    return false
}
