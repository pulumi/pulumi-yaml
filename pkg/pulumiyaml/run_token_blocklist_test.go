// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that we can collect errors for disallowed resources.
func TestBlocklistPulumi(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  dockerImageFull:
    type: docker:image:Image
  dockerImageShort:
    type: docker:Image
  kubeCustomResource:
    type: kubernetes:apiextensions.k8s.io:CustomResource
  kubeKustomizeDir:
    type: kubernetes:kustomize:Directory
  kubeYamlConfigFile:
    type: kubernetes:yaml:ConfigFile
  kubeYamlConfigGroup:
    type: kubernetes:yaml:ConfigGroup
  helmChartV2:
    type: kubernetes:helm.sh/v2:Chart
  helmChartV3:
    type: kubernetes:helm.sh/v3:Chart
`
	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
	assert.True(t, diags.HasErrors())
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	expectedErrors := []string{
		"<stdin>:5:11: error resolving type of resource dockerImageFull: The resource type [docker:image:Image] is not supported in YAML at this time, see: https://github.com/pulumi/pulumi-docker/issues/132",
		"<stdin>:7:11: error resolving type of resource dockerImageShort: The resource type [docker:Image] is not supported in YAML at this time, see: https://github.com/pulumi/pulumi-docker/issues/132",
		"<stdin>:9:11: error resolving type of resource kubeCustomResource: The resource type [kubernetes:apiextensions.k8s.io:CustomResource] is not supported in YAML at this time, see: https://github.com/pulumi/pulumi-kubernetes/issues/1971",
		"<stdin>:11:11: error resolving type of resource kubeKustomizeDir: The resource type [kubernetes:kustomize:Directory] is not supported in YAML at this time, see: https://github.com/pulumi/pulumi-kubernetes/issues/1971",
		"<stdin>:13:11: error resolving type of resource kubeYamlConfigFile: The resource type [kubernetes:yaml:ConfigFile] is not supported in YAML at this time, see: https://github.com/pulumi/pulumi-kubernetes/issues/1971",
		"<stdin>:15:11: error resolving type of resource kubeYamlConfigGroup: The resource type [kubernetes:yaml:ConfigGroup] is not supported in YAML at this time, see: https://github.com/pulumi/pulumi-kubernetes/issues/1971",
		"<stdin>:17:11: error resolving type of resource helmChartV2: Helm Chart resources are not supported in YAML, consider using the Helm Release resource instead: https://www.pulumi.com/registry/packages/kubernetes/api-docs/helm/v3/release/",
		"<stdin>:19:11: error resolving type of resource helmChartV3: Helm Chart resources are not supported in YAML, consider using the Helm Release resource instead: https://www.pulumi.com/registry/packages/kubernetes/api-docs/helm/v3/release/",
	}
	assert.ElementsMatch(t, expectedErrors, diagStrings)
	assert.Len(t, diagStrings, len(expectedErrors))
}
