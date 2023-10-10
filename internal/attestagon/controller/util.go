package controller

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

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

func (c *Controller) ReadyForProcessing(pod *corev1.Pod) (bool, Artifact) {
	fmt.Println(c.Artifacts)
	for i := 0; i < len(c.Artifacts); i++ {
		if (pod.Status.Phase == "Running" || pod.Status.Phase == "Pending") && pod.Labels["attestagon.io/artifact"] == c.Artifacts[i].Name && c.Artifacts[i].Name != "" && pod.Annotations["attestagon.io/attested"] != "true" {
			return true, c.Artifacts[i]
		}
	}

	return false, Artifact{}
}
