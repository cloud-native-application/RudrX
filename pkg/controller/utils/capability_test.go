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

package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const TestDir = "testdata/definition"

func TestGetOpenAPISchema(t *testing.T) {
	type want struct {
		data string
		err  error
	}
	cases := map[string]struct {
		reason string
		name   string
		data   string
		want   want
	}{
		"parameter in cue is a structure type,": {
			reason: "Prepare a normal parameter cue file",
			name:   "workload1",
			data: `
project: {
	name: string
}

	parameter: {
	min: int
}
`,
			want: want{data: "{\"properties\":{\"min\":{\"title\":\"min\",\"type\":\"integer\"}},\"required\":[\"min\"],\"type\":\"object\"}", err: nil},
		},
		"parameter in cue is a dict type,": {
			reason: "Prepare a normal parameter cue file",
			name:   "workload2",
			data: `
annotations: {
	for k, v in parameter {
		"\(k)": v
	}
}

parameter: [string]: string
`,
			want: want{data: "{\"additionalProperties\":{\"type\":\"string\"},\"type\":\"object\"}", err: nil},
		},
		"parameter in cue is a string type,": {
			reason: "Prepare a normal parameter cue file",
			name:   "workload3",
			data: `
annotations: {
	"test":parameter
}

   parameter:string
`,
			want: want{data: "{\"type\":\"string\"}", err: nil},
		},
		"cue doesn't contain parameter section": {
			reason: "Prepare a cue file which doesn't contain `parameter` section",
			name:   "invalidWorkload",
			data: `
project: {
	name: string
}

noParameter: {
	min: int
}
`,
			want: want{data: "", err: fmt.Errorf("capability invalidWorkload doesn't contain section `parameter`")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			schematic := &common.Schematic{
				CUE: &common.CUE{
					Template: tc.data,
				},
			}
			capability, _ := appfile.ConvertTemplateJSON2Object(tc.name, nil, schematic)
			schema, err := getOpenAPISchema(capability, pd)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetOpenAPISchema(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if tc.want.err == nil {
				assert.Equal(t, string(schema), tc.want.data)
			}
		})
	}
}

func TestFixOpenAPISchema(t *testing.T) {
	cases := map[string]struct {
		inputFile string
		fixedFile string
	}{
		"StandardWorkload": {
			inputFile: "webservice.json",
			fixedFile: "webserviceFixed.json",
		},
		"ShortTagJson": {
			inputFile: "shortTagSchema.json",
			fixedFile: "shortTagSchemaFixed.json",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			swagger, _ := openapi3.NewSwaggerLoader().LoadSwaggerFromFile(filepath.Join(TestDir, tc.inputFile))
			schema := swagger.Components.Schemas[cue.ParameterTag].Value
			fixOpenAPISchema("", schema)
			fixedSchema, _ := schema.MarshalJSON()
			expectedSchema, _ := ioutil.ReadFile(filepath.Join(TestDir, tc.fixedFile))
			assert.Equal(t, string(fixedSchema), string(expectedSchema))
		})
	}
}

func TestGenerateOpenAPISchemaFromCapabilityParameter(t *testing.T) {
	var invalidWorkloadName = "IAmAnInvalidWorkloadDefinition"
	capabilityDir, _ := system.GetCapabilityDir()
	if _, err := os.Stat(capabilityDir); err != nil && os.IsNotExist(err) {
		os.MkdirAll(capabilityDir, 0755)
	}

	type want struct {
		data []byte
		err  error
	}

	cases := map[string]struct {
		reason     string
		capability types.Capability
		want       want
	}{
		"GenerateOpenAPISchemaFromInvalidCapability": {
			reason:     "generate OpenAPI schema for an invalid Workload/Trait",
			capability: types.Capability{Name: invalidWorkloadName},
			want:       want{data: nil, err: fmt.Errorf("capability IAmAnInvalidWorkloadDefinition doesn't contain section `parameter`")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := generateOpenAPISchemaFromCapabilityParameter(tc.capability, pd)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetDefinition(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.data, got); diff != "" {
				t.Errorf("\n%s\ngetDefinition(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestNewCapabilityComponentDef(t *testing.T) {
	terraform := &common.Terraform{
		Configuration: "test",
	}
	componentDefinition := &v1beta1.ComponentDefinition{
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &common.Schematic{
				Terraform: terraform,
			},
		},
	}
	def := NewCapabilityComponentDef(componentDefinition)
	assert.Equal(t, def.WorkloadType, util.TerraformDef)
	assert.Equal(t, def.Terraform, terraform)
}

func TestGetOpenAPISchemaFromTerraformComponentDefinition(t *testing.T) {
	configuration := `
module "rds" {
  source = "terraform-alicloud-modules/rds/alicloud"
  engine = "MySQL"
  engine_version = "8.0"
  instance_type = "rds.mysql.c1.large"
  instance_storage = "20"
  instance_name = var.instance_name
  account_name = var.account_name
  password = var.password
}

output "DB_NAME" {
  value = module.rds.this_db_instance_name
}
output "DB_USER" {
  value = module.rds.this_db_database_account
}
output "DB_PORT" {
  value = module.rds.this_db_instance_port
}
output "DB_HOST" {
  value = module.rds.this_db_instance_connection_string
}
output "DB_PASSWORD" {
  value = module.rds.this_db_instance_port
}

variable "instance_name" {
  description = "RDS instance name"
  type = string
  default = "poc"
}

variable "account_name" {
  description = "RDS instance user account name"
  type = "string"
  default = "oam"
}

variable "password" {
  description = "RDS instance account password"
  type = "string"
  default = "xxx"
}

variable "intVar" {
  type = "number"
}

variable "boolVar" {
  type = "bool"
}

variable "listVar" {
  type = "list"
}

variable "mapVar" {
  type = "map"
}`

	schema, err := GetOpenAPISchemaFromTerraformComponentDefinition(configuration)
	assert.NilError(t, err)
	data := string(schema)
	assert.Equal(t, strings.Contains(data, "account_name"), true)
	assert.Equal(t, strings.Contains(data, "intVar"), true)
}
