package controller

import (
   "os"
   "gopkg.in/yaml.v2"
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
