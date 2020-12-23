package main

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/structured-merge-diff/v4/fieldpath"
	"sigs.k8s.io/structured-merge-diff/v4/typed"
	"sigs.k8s.io/structured-merge-diff/v4/value"
	"sigs.k8s.io/yaml"
)

type filter interface {
	fields(obj *unstructured.Unstructured) error
	gvrs(gvrs []schema.GroupVersionResource) []schema.GroupVersionResource
	shouldNotWrite(obj *unstructured.Unstructured) bool
}

type dumpFilter struct {
	fieldFiltersSet *fieldpath.Set
}

func newFilter() (filter, error) {
	fieldFiltersSet, err := getFieldFiltersSet()
	if err != nil {
		return nil, err
	}

	dumpFilter := &dumpFilter{
		fieldFiltersSet: fieldFiltersSet,
	}
	return dumpFilter, nil
}

func getFieldFiltersSet() (*fieldpath.Set, error) {
	path := filepath.Join("..", "dump-files", "fieldFilters.yaml")
	fileData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	jsonData, err := yaml.YAMLToJSON(fileData)
	if err != nil {
		return nil, err
	}

	val, err := value.FromJSON(jsonData)
	if err != nil {
		return nil, err
	}
	return fieldpath.SetFromValue(val), nil
}

func (f *dumpFilter) fields(obj *unstructured.Unstructured) error {
	allFieldsV1Set, err := f.getAllFieldsV1Set(obj.GetManagedFields())
	if err != nil {
		return err
	}
	itemsToBeRemoved := f.fieldFiltersSet.Union(allFieldsV1Set)

	typedValue, err := typed.DeducedParseableType.FromUnstructured(obj.Object)
	if err != nil {
		return err
	}
	typedValue = typedValue.RemoveItems(itemsToBeRemoved)

	objectResult, ok := typedValue.AsValue().Unstructured().(map[string]interface{})
	if !ok {
		return errors.New("result is not of type map[string]interface{}")
	}
	obj.Object = objectResult

	return nil
}

func (f *dumpFilter) getAllFieldsV1Set(managedFields []metav1.ManagedFieldsEntry) (*fieldpath.Set, error) {
	allFieldsV1Set := &fieldpath.Set{}
	for _, managedField := range managedFields {
		// TODO improve which fields should be kept
		if strings.HasPrefix(managedField.Manager, "kubectl") {
			continue
		}

		data, err := managedField.FieldsV1.MarshalJSON()
		if err != nil {
			return nil, err
		}
		reader := strings.NewReader(string(data))

		fieldsV1Set := &fieldpath.Set{}
		if err := fieldsV1Set.FromJSON(reader); err != nil {
			return nil, err
		}
		allFieldsV1Set = allFieldsV1Set.Union(fieldsV1Set)
	}
	return allFieldsV1Set, nil
}

func (f *dumpFilter) gvrs(gvrs []schema.GroupVersionResource) []schema.GroupVersionResource {
	filteredGvrs := []schema.GroupVersionResource{}
	for _, gvr := range gvrs {
		if gvr.Resource != "events" && gvr.Resource != "nodes" {
			filteredGvrs = append(filteredGvrs, gvr)
		}
	}
	return filteredGvrs
}

func (f *dumpFilter) shouldNotWrite(obj *unstructured.Unstructured) bool {
	return f.shouldIgnoreNamespace(obj) || f.isOwned(obj)
}

func (f *dumpFilter) shouldIgnoreNamespace(obj *unstructured.Unstructured) bool {
	return obj.GetNamespace() == "kube-node-lease"
}

func (f *dumpFilter) isOwned(obj *unstructured.Unstructured) bool {
	return len(obj.GetOwnerReferences()) > 0
}
