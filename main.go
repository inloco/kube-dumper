package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	repositoryURL := os.Getenv("REPOSITORY_URL")
	refreshGvrsTimeInMinutes, err := strconv.ParseInt(os.Getenv("REFRESH_GVRS_TIME_IN_MINUTES"), 10, 64)
	if err != nil {
		log.Panic(err)
	}

	clusterConfig, err := getClusterConfig()
	if err != nil {
		log.Panic(err)
	}

	log.Print("Reset current directory")
	fileManager := newFileManager()
	if err := fileManager.resetCurrentDirectory(); err != nil {
		log.Panic(err)
	}

	log.Print("Clone git repository from ", repositoryURL)
	repository, err := newRepository(repositoryURL)
	if err != nil {
		log.Panic(err)
	}

	cypher := newCypher()
	discovery := newDiscovery(clusterConfig)
	filter, err := newFilter()
	if err != nil {
		log.Panic(err)
	}

	managerConfig := &managerConfig{
		Discovery:       discovery,
		Cypher:          cypher,
		FileManager:     fileManager,
		Filter:          filter,
		Repository:      repository,
		RefreshGvrsTime: time.Duration(refreshGvrsTimeInMinutes) * time.Minute,
		ClusterConfig:   clusterConfig,
	}
	manager, err := newManager(managerConfig)
	if err != nil {
		log.Panic(err)
	}

	if err := manager.manage(); err != nil {
		log.Panic(err)
	}
}

func getClusterConfig() (*rest.Config, error) {
	log.Println("Load in Custer Client Configuration")
	clusterConfig, inClusterConfigErr := rest.InClusterConfig()
	if inClusterConfigErr == nil {
		return clusterConfig, nil
	}
	log.Println("Failed to load in Custer Client Configuration")

	log.Println("Load out of Custer Client Configuration")
	kubeConfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	clusterConfig, outClusterConfigErr := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if outClusterConfigErr != nil {
		return nil, fmt.Errorf("Failed to load cluster config: [in: %s, out: %s]", inClusterConfigErr, outClusterConfigErr)
	}
	return clusterConfig, nil
}
