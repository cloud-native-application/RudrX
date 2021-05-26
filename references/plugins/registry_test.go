/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package plugins

import (
	"context"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
)

func TestRegistry(t *testing.T) {
	testAddon := "init-container"
	regName := "testReg"
	localPath, err := filepath.Abs("../../e2e/plugin/testdata")
	assert.Nil(t, err)

	cases := map[string]struct {
		url       string
		expectReg Registry
	}{
		"github registry": {
			url:       "https://github.com/oam-dev/catalog/tree/master/registry",
			expectReg: GithubRegistry{},
		},
		"local registry": {
			url:       "file://" + localPath,
			expectReg: LocalRegistry{},
		},
	}

	for _, c := range cases {
		registry, err := NewRegistry(context.Background(), "", regName, c.url)
		assert.NoError(t, err, regName)
		assert.IsType(t, c.expectReg, registry, regName)

		caps, err := registry.ListCaps()
		assert.NoError(t, err, regName)
		assert.NotEmpty(t, caps, regName)

		capability, data, err := registry.GetCap(testAddon)
		assert.NoError(t, err, regName)
		assert.NotNil(t, capability, testAddon)
		assert.NotNil(t, data, testAddon)
	}
}
