package main

import (
	"errors"
	"fmt"

	"go.mozilla.org/sops/v3"
	"go.mozilla.org/sops/v3/aes"
	"go.mozilla.org/sops/v3/cmd/sops/common"
	"go.mozilla.org/sops/v3/cmd/sops/formats"
	"go.mozilla.org/sops/v3/config"
	"go.mozilla.org/sops/v3/decrypt"
	"go.mozilla.org/sops/v3/keyservice"
	"go.mozilla.org/sops/v3/version"
)

type cypher interface {
	decrypt(path string) ([]byte, error)
	encrypt(path string, data []byte) ([]byte, error)
}

type dumpCypher struct{}

func newCypher() cypher {
	return &dumpCypher{}
}

func (m *dumpCypher) decrypt(path string) ([]byte, error) {
	return decrypt.File(path, "yaml")
}

func (m *dumpCypher) encrypt(path string, data []byte) ([]byte, error) {
	store := common.StoreForFormat(formats.Yaml)
	branches, err := store.LoadPlainFile(data)
	if err != nil {
		return nil, err
	}

	sopsConfig, err := config.LoadCreationRuleForFile(".sops.yaml", path, nil)
	if err != nil {
		return nil, err
	}

	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			KeyGroups:         sopsConfig.KeyGroups,
			UnencryptedSuffix: sopsConfig.UnencryptedSuffix,
			EncryptedSuffix:   sopsConfig.EncryptedSuffix,
			UnencryptedRegex:  sopsConfig.UnencryptedRegex,
			EncryptedRegex:    sopsConfig.EncryptedRegex,
			Version:           version.Version,
			ShamirThreshold:   sopsConfig.ShamirThreshold,
		},
		FilePath: path,
	}

	keyServices := []keyservice.KeyServiceClient{keyservice.NewLocalClient()}
	dataKey, errs := tree.GenerateDataKeyWithKeyServices(keyServices)
	if len(errs) > 0 {
		return nil, errors.New(fmt.Sprint("Could not generate data key:", errs))
	}

	encryptTreeOpts := common.EncryptTreeOpts{
		DataKey: dataKey,
		Tree:    &tree,
		Cipher:  aes.NewCipher(),
	}
	err = common.EncryptTree(encryptTreeOpts)
	if err != nil {
		return nil, err
	}

	return store.EmitEncryptedFile(tree)
}
