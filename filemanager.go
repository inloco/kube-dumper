package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type fileManager interface {
	resetCurrentDirectory() error
	writeFile(path string, data []byte) error
	deleteFile(path string) error
}

type dumpFileManager struct{}

func newFileManager() fileManager {
	return &dumpFileManager{}
}

func (m *dumpFileManager) resetCurrentDirectory() error {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := os.RemoveAll(file.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (m *dumpFileManager) writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return ioutil.WriteFile(path, data, 0644)
}

func (m *dumpFileManager) deleteFile(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}

	m.removeEmptyDirectories(path)
	return nil
}

func (m *dumpFileManager) removeEmptyDirectories(path string) {
	var err error
	for err == nil {
		path = filepath.Dir(path)
		err = os.Remove(path)
	}
}
