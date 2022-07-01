package main

import (
	"bufio"
	"context"
	"encoding/json"

	// "encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/cilium/tetragon/api/v1/tetragon"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)


func main() {

  if err := initialise(); err != nil {
    panic(err)
  }

}

func initialise() error {

  // Get Cluster Config
  path := os.Getenv("HOME") + "/.kube/config"
  config, err := clientcmd.BuildConfigFromFlags("", path)
  if err != nil {
    fmt.Printf("ERROR: Failed to get cluster config: %s", string(err.Error()))
    return err
  }

  clientset, err := kubernetes.NewForConfig(config)
  if err != nil {
    fmt.Printf("ERROR: Failed to get cluster config: %s", string(err.Error())) 
    return err
  }
  listOpts := metav1.ListOptions{
    LabelSelector: "app.kubernetes.io/name=tetragon",
  }

  for {

    req, err := clientset.CoreV1().Pods("kube-system").List(context.TODO(), listOpts)
    if err != nil {
      fmt.Printf("ERROR: Failed to get tetragon pod names: %s", string(err.Error()))
      return err
    }

    // Configuring Pod Log Options
    podLogOpts := corev1.PodLogOptions{
      Container: "export-stdout",
      Follow: true,
    }
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    wg := sync.WaitGroup{}

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

      wg.Add(1)
      go podLogProcessor(ctx, &wg, clientset, req.Items[i].Name, "kube-system", podLogOpts) 
    }

    wg.Wait()
  }
}

func podLogProcessor(syncCtx context.Context, wg *sync.WaitGroup, clientset *kubernetes.Clientset, podName string, namespace string, podLogOpts corev1.PodLogOptions) {
  
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
                return
              }
              panic(err)
            } else {
                var response *tetragon.GetEventsResponse 
                json.Unmarshal(outputLine, &response)

                processEvent(response)

            }
          }
  }
}

func processEvent(response *tetragon.GetEventsResponse) (error) {
  switch response.Event.(type) {
  case *tetragon.GetEventsResponse_ProcessExec:
          exec := response.GetProcessExec()
          findBuild(exec)
  case *tetragon.GetEventsResponse_ProcessExit:
          return nil
  case *tetragon.GetEventsResponse_ProcessKprobe:
          // kprobe := response.GetProcessKprobe()
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

func findBuild(event *tetragon.ProcessExec) error {

  if event.Process.Pod.Namespace == "tekton-pipelines" {
    fmt.Printf("We've found one! Pod %s is in the %s namespace! \n\n", event.Process.Pod.Name, event.Process.Pod.Namespace)
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
