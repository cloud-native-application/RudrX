# Use Helm To Extend a Component type

This documentation explains how to use Helm chart to define an application component.

Before reading this part, please make sure you've learned [the definition and template concepts](../platform-engineers/definition-and-templates.md).

## Prerequisite

* [fluxcd/flux2](../install.md#3-optional-install-flux2), make sure you have installed the flux2 in the [installation guide](https://kubevela.io/#/en/install).

## Write ComponentDefinition

Here is an example `ComponentDefinition` about how to use Helm as schematic module.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webapp-chart
  annotations:
    definition.oam.dev/description: helm chart for webapp
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    helm:
      release:
        chart:
          spec:
            chart: "podinfo"
            version: "5.1.4"
      repository:
        url: "http://oam.dev/catalog/"
```

Just like using CUE as schematic module, we also have some rules and contracts to use helm chart as schematic module.

- `.spec.workload` is required to indicate the main workload(apiVersion/Kind) in your Helm chart.
Only one workload allowed in one helm chart.
For example, in our sample chart, the core workload is `deployments.apps/v1`, other resources will also be deployed but mechanism of KubeVela won't work for them.
- `.spec.schematic.helm` contains information of Helm release & repository.

There are two fields `release` and `repository` in the `.spec.schematic.helm` section, these two fields align with the APIs of `fluxcd/flux2`. Spec of `release` aligns with [`HelmReleaseSpec`](https://github.com/fluxcd/helm-controller/blob/main/docs/api/helmrelease.md) and spec of `repository` aligns with [`HelmRepositorySpec`](https://github.com/fluxcd/source-controller/blob/main/docs/api/source.md#source.toolkit.fluxcd.io/v1beta1.HelmRepository).
In a word, just like the fields shown in the sample, the helm schematic module describes a specific Helm chart release and its repository.

## Create an Application using the helm based ComponentDefinition

Here is an example `Application`.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: myapp
  namespace: default
spec:
  components:
    - name: demo-podinfo 
      type: webapp-chart 
      properties: 
        image:
          tag: "5.1.2"
```

Helm module workload will use data in `properties` as [Helm chart values](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml).
You can learn the schema of settings by reading the `README.md` of the Helm
chart, and the schema are totally align with
[`values.yaml`](https://github.com/captainroy-hy/podinfo/blob/master/charts/podinfo/values.yaml)
of the chart.  

Helm v3 has [support to validate
values](https://helm.sh/docs/topics/charts/#schema-files) in a chart's
values.yaml file with JSON schemas.  
Vela will try to fetch the `values.schema.json` file from the Chart archive and
[save the schema into a
ConfigMap](https://kubevela.io/#/en/platform-engineers/openapi-v3-json-schema.md)
which can be consumed latter through UI or CLI.  
If `values.schema.json` is not provided by the Chart author, Vela will generate a
OpenAPI-v3 JSON schema based on the `values.yaml` file automatically.  

Deploy the application and after several minutes (it takes time to fetch Helm chart from the repo, render and install), you can check the Helm release is installed.
```shell
$ helm ls -A
myapp-demo-podinfo	default  	1 	2021-03-05 02:02:18.692317102 +0000 UTC	deployed	podinfo-5.1.4   	5.1.4
```
Check the deployment defined in the chart has been created successfully.
```shell
$ kubectl get deploy
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
myapp-demo-podinfo   1/1     1            1           66m
```

Check the values(`image.tag = 5.1.2`) from application's `settings` are assigned to the chart.
```shell
$ kubectl get deployment myapp-demo-podinfo -o json | jq '.spec.template.spec.containers[0].image'
"ghcr.io/stefanprodan/podinfo:5.1.2"
```
