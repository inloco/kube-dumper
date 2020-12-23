package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

type watcher interface {
	stop()
	start() error
}

type dumpWatcher struct {
	clusterClient dynamic.Interface
	cypher        cypher
	fileManager   fileManager
	filter        filter
	gvr           schema.GroupVersionResource
	logger        *log.Logger
	repository    repository
	resourcePaths map[string]bool
	watch         watch.Interface
}

type watcherConfig struct {
	clusterClient dynamic.Interface
	cypher        cypher
	fileManager   fileManager
	filter        filter
	gvr           schema.GroupVersionResource
	repository    repository
}

func newWatcher(config *watcherConfig) (watcher, error) {
	logger := log.New(os.Stderr, fmt.Sprintf("%s: ", config.gvr.GroupResource().String()), log.LstdFlags)
	resourcePaths, err := getResourcePaths(config.gvr)
	if err != nil {
		return nil, err
	}

	dumpWatcher := &dumpWatcher{
		clusterClient: config.clusterClient,
		cypher:        config.cypher,
		fileManager:   config.fileManager,
		filter:        config.filter,
		gvr:           config.gvr,
		logger:        logger,
		repository:    config.repository,
		resourcePaths: resourcePaths,
	}
	return dumpWatcher, err
}

func getResourcePaths(gvr schema.GroupVersionResource) (map[string]bool, error) {
	watcherGroupResource := gvr.GroupResource().String()

	resourcePaths := map[string]bool{}
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		groupResource := filepath.Base(filepath.Dir(path))
		if groupResource == watcherGroupResource {
			resourcePaths[path] = true
		}
		return nil
	})
	return resourcePaths, err
}

func (w *dumpWatcher) stop() {
	w.watch.Stop()
}

func (w *dumpWatcher) start() error {
	w.logger.Print("Start watching")
	defer w.logger.Print("Stop watching")

	watch, err := w.clusterClient.Resource(w.gvr).Watch(metav1.ListOptions{})
	if err != nil {
		return err
	}
	w.watch = watch

	if err := w.reconcileResources(); err != nil {
		return err
	}

	return w.listenWatchEvents()
}

func (w *dumpWatcher) reconcileResources() error {
	w.logger.Print("Process reconciliation")

	updatedResourcePaths, err := w.updateResources()
	if err != nil {
		return err
	}

	commitMsg := fmt.Sprintf("reconcile: %s", w.gvr.GroupResource().String())

	somethingChanged, err := w.repository.addCommitAndPush(commitMsg, updatedResourcePaths)
	if err != nil {
		return err
	}

	if somethingChanged {
		w.logger.Printf("Commit: \"%s\"", commitMsg)
		return nil
	}

	w.logger.Print("Nothing changed")
	return nil
}

func (w *dumpWatcher) updateResources() ([]string, error) {
	list, err := w.clusterClient.Resource(w.gvr).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	deletedResourcePaths := w.resourcePaths
	w.resourcePaths = map[string]bool{}
	updatedResourcePaths := []string{}

	for _, item := range list.Items {
		resourcePath, err := w.writeResource(&item)
		if err != nil {
			return nil, err
		}

		if resourcePath == "" {
			continue
		}

		delete(deletedResourcePaths, resourcePath)
		w.resourcePaths[resourcePath] = true
		updatedResourcePaths = append(updatedResourcePaths, resourcePath)
	}

	for deletedResourcePath := range deletedResourcePaths {
		if err := w.fileManager.deleteFile(deletedResourcePath); err != nil {
			return nil, err
		}
		updatedResourcePaths = append(updatedResourcePaths, deletedResourcePath)
	}

	return updatedResourcePaths, nil
}

func (w *dumpWatcher) listenWatchEvents() error {
	for e := range w.watch.ResultChan() {
		obj, ok := e.Object.(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("type assertion failed")
		}

		eventType := strings.ToLower(fmt.Sprintf("%s", e.Type))
		w.logger.Printf("Process event: %s %s/%s", eventType, w.getNamespace(obj), obj.GetName())

		resourcePath, err := w.processEvent(e.Type, obj)
		if err != nil {
			return err
		}

		if resourcePath == "" {
			w.logger.Print("Resource ignored")
			continue
		}

		commitMsg := fmt.Sprintf("%s: %s", eventType, resourcePath)

		committed, err := w.repository.addCommitAndPush(commitMsg, []string{resourcePath})
		if err != nil {
			return err
		}

		if committed {
			w.logger.Printf("Commit: \"%s\"", commitMsg)
			continue
		}

		w.logger.Print("Nothing changed")
	}
	return nil
}

func (w *dumpWatcher) processEvent(eventType watch.EventType, obj *unstructured.Unstructured) (string, error) {
	switch eventType {
	case watch.Added, watch.Modified:
		return w.writeResource(obj)
	case watch.Deleted:
		return w.deleteResource(obj)
	default:
		return "", fmt.Errorf("unable to handle event of type: %s", eventType)
	}
}

func (w *dumpWatcher) writeResource(obj *unstructured.Unstructured) (string, error) {
	if w.filter.shouldNotWrite(obj) {
		return "", nil
	}

	if err := w.filter.fields(obj); err != nil {
		return "", err
	}

	resourcePath, data, err := w.getPathAndData(obj)
	if err != nil || data == nil {
		return resourcePath, err
	}

	if err := w.fileManager.writeFile(resourcePath, data); err != nil {
		return "", err
	}

	w.resourcePaths[resourcePath] = true

	return resourcePath, nil
}

func (w *dumpWatcher) getPathAndData(obj *unstructured.Unstructured) (string, []byte, error) {
	data, err := yaml.Marshal(obj.Object)
	if err != nil {
		return "", nil, err
	}

	path := w.getPath(obj)

	if w.gvr.GroupResource().String() == "secrets" {
		data, err = w.cypher.encrypt(path, data)
		if err != nil {
			return "", nil, err
		}
	}
	return path, data, nil
}

func (w *dumpWatcher) deleteResource(obj *unstructured.Unstructured) (string, error) {
	path := w.getPath(obj)
	if err := w.fileManager.deleteFile(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}

	delete(w.resourcePaths, path)

	return path, nil
}

func (w *dumpWatcher) getPath(obj *unstructured.Unstructured) string {
	namespace := w.getNamespace(obj)
	groupResource := w.gvr.GroupResource().String()
	fileName := obj.GetName() + ".yaml"
	return filepath.Join(namespace, groupResource, fileName)
}

func (w *dumpWatcher) getNamespace(obj *unstructured.Unstructured) string {
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = "_"
	}
	return namespace
}
