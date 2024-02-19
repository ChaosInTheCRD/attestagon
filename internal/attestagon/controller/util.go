package controller

import (
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

func (c *Controller) ReadyForProcessing(pod *corev1.Pod) *Artifact {
	for i := 0; i < len(c.artifacts); i++ {
		if pod.Status.Phase == "Succeeded" && pod.Annotations["attestagon.io/artifact"] == c.artifacts[i].Name && c.artifacts[i].Name != "" && pod.Annotations["attestagon.io/attested"] != "true" {
			return &c.artifacts[i]
		}
	}

	return nil
}
