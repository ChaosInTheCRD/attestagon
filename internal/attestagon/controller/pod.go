package controller

import (
   "fmt"
   "context"
   "encoding/json"
   corev1 "k8s.io/api/core/v1"
   "github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
   "github.com/in-toto/in-toto-golang/in_toto"
)

type PodTerminationMessage struct {
  Key string `json:"key"`
  Value string `json:"value"`
}


func (c *Controller) ProcessPod(ctx context.Context, pod *corev1.Pod) error {

  for i:= 0; i < len(c.artifacts); i++ {
    if pod.Annotations["attestagon.io/artifact"] == c.artifacts[i].Name && c.artifacts[i].Name != "" && pod.Status.Phase == "Succeeded" && pod.Annotations["attestagon.io/attested"] != "true" {
        fmt.Println("Processing pod", pod.Name)

        var predicate predicate.Predicate
        predicate.Pod.Name = pod.Name
        predicate.Pod.Namespace = pod.Namespace
        
        metadata, err := c.GetRuntimeMetadata(ctx, predicate, pod)
        if err != nil {
           c.log.Error(err, "Failed to get tetragon runtime metadata: ")
        }

        for i := range(metadata) {
           predicate.ProcessEvent(&metadata[i])
        }

        statement := in_toto.Statement{
          StatementHeader: in_toto.StatementHeader{
            Type: "https://in-toto.io/Statement/v0.1",
            PredicateType: "https://attestagon.io/provenance/v0.1",
            Subject: []in_toto.Subject{{Name: c.artifacts[i].Name}},
          },
          Predicate: predicate,
        }

        for _, status := range pod.Status.ContainerStatuses {
          message := []PodTerminationMessage{}
          json.Unmarshal([]byte(status.State.Terminated.Message), &message)

          for i := 0; i < len(message); i++ {
            if message[i].Key == "digest" {
              // _, err := godigest.Parse(message[i].Key)
              // if err != nil {
                // return fmt.Errorf("Digest (%s) found in termination message for container %s in pod %s not valid digest:", message[i].Value, status.Name, pod.Name)
              // }

              fmt.Printf("Ready to sign and push the attestation!\n")
              ctx := context.TODO()
              imageRef := fmt.Sprintf("%s@%s", c.artifacts[i].Ref, message[i].Value)
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
