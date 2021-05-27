package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type manager interface {
	manage() error
}

type dumpManager struct {
	clusterClient   dynamic.Interface
	cypher          cypher
	discovery       discovery
	fileManager     fileManager
	filter          filter
	refreshGvrsTime time.Duration
	repository      repository
	watchers        map[schema.GroupVersionResource]watcher
	watchersLock    sync.RWMutex
}

type managerConfig struct {
	Discovery       discovery
	Cypher          cypher
	FileManager     fileManager
	Filter          filter
	RefreshGvrsTime time.Duration
	Repository      repository
	ClusterConfig   *rest.Config
}

func newManager(config *managerConfig) (manager, error) {
	clusterClient, err := dynamic.NewForConfig(config.ClusterConfig)
	if err != nil {
		return nil, err
	}

	dumpManager := &dumpManager{
		clusterClient:   clusterClient,
		cypher:          config.Cypher,
		discovery:       config.Discovery,
		fileManager:     config.FileManager,
		filter:          config.Filter,
		refreshGvrsTime: config.RefreshGvrsTime,
		repository:      config.Repository,
		watchers:        map[schema.GroupVersionResource]watcher{},
		watchersLock:    sync.RWMutex{},
	}
	return dumpManager, nil
}

func (m *dumpManager) manage() error {
	for {
		log.Print("Discover server resources")
		gvrs, err := m.discovery.discover()
		if err != nil {
			return err
		}
		gvrs = m.filter.gvrs(gvrs)

		if err := m.start(gvrs); err != nil {
			return err
		}

		log.Print("Remove untracked GVRs")
		if err = m.removeUntrackedGvrs(gvrs); err != nil {
			return err
		}
		log.Print("All untracked GVRs removed")

		time.Sleep(m.refreshGvrsTime)
	}
}

func (m *dumpManager) start(gvrs []schema.GroupVersionResource) error {
	m.watchersLock.Lock()
	defer m.watchersLock.Unlock()

	for _, gvr := range gvrs {
		if _, ok := m.watchers[gvr]; ok {
			continue
		}

		config := &watcherConfig{
			cypher:        m.cypher,
			fileManager:   m.fileManager,
			filter:        m.filter,
			repository:    m.repository,
			clusterClient: m.clusterClient,
			gvr:           gvr,
		}
		watcher, err := newWatcher(config)
		if err != nil {
			return err
		}
		m.watchers[gvr] = watcher

		go m.startWatcher(gvr)
	}
	return nil
}

func (m *dumpManager) removeUntrackedGvrs(gvrs []schema.GroupVersionResource) error {
	trackedGroupResources := map[string]bool{}
	for _, gvr := range gvrs {
		trackedGroupResources[gvr.GroupResource().String()] = true
	}

	untrackedGroupResources, err := m.removeUntrackedGroupResources(trackedGroupResources)
	if err != nil {
		return err
	}
	return m.commitUntrackedGroupResources(untrackedGroupResources)
}

func (m *dumpManager) removeUntrackedGroupResources(trackedGroupResources map[string]bool) (map[string][]string, error) {
	untrackedGroupResources := map[string][]string{}
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasPrefix(path, ".git") || filepath.Dir(path) == "." {
			return err
		}

		groupResource := filepath.Base(filepath.Dir(path))
		if _, ok := trackedGroupResources[groupResource]; !ok {
			if err := m.fileManager.deleteFile(path); err != nil {
				return err
			}
			untrackedGroupResources[groupResource] = append(untrackedGroupResources[groupResource], path)
		}
		return nil
	})
	return untrackedGroupResources, err
}

func (m *dumpManager) commitUntrackedGroupResources(untrackedGroupResources map[string][]string) error {
	for untrackedGroupResource, resourcePaths := range untrackedGroupResources {
		commitMsg := fmt.Sprintf("reconcile: %s", untrackedGroupResource)

		committed, err := m.repository.addCommitAndPush(commitMsg, resourcePaths)
		if err != nil {
			return err
		}

		if committed {
			log.Printf("Commit: \"%s\"", commitMsg)
		}
	}
	return nil
}

func (m *dumpManager) startWatcher(gvr schema.GroupVersionResource) {
	for {
		m.watchersLock.RLock()
		watcher, ok := m.watchers[gvr]
		m.watchersLock.RUnlock()
		if !ok {
			break
		}

		if err := watcher.start(); err != nil {
			if !errors.IsNotFound(err) {
				log.Panic(err)
			}

			m.stop([]schema.GroupVersionResource{gvr})
		}
	}
}

func (m *dumpManager) stop(gvrs []schema.GroupVersionResource) {
	m.watchersLock.Lock()
	defer m.watchersLock.Unlock()

	for _, gvr := range gvrs {
		if watcher, ok := m.watchers[gvr]; ok {
			delete(m.watchers, gvr)
			watcher.stop()
		}
	}
}
