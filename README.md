# Kubernetes Jobs Manager Operator

- [Kubernetes Jobs Manager Operator](#kubernetes-jobs-manager-operator)
  - [Description](#description)
  - [Getting Started](#getting-started)
    - [Installation with helm](#installation-with-helm)
    - [Prerequisites for local runs](#prerequisites-for-local-runs)
    - [Jobs configuration](#jobs-configuration)
    - [How does it look in practice?](#how-does-it-look-in-practice)
    - [Things to remember](#things-to-remember)
    - [Available params](#available-params)
    - [Kustomization and references](#kustomization-and-references)
    - [Running on the cluster](#running-on-the-cluster)
      - [Manual installation](#manual-installation)
      - [Manually uninstall CRDs](#manually-uninstall-crds)
    - [Manually undeploy controller](#manually-undeploy-controller)
    - [How it works](#how-it-works)
  - [License](#license)


## Description
This operator is responsible for managing the lifecycle of complicated workflows which consist of multiple jobs and making their management easy, without need for dozens of yaml files and doing magic with ordering.

## Getting Started

### Installation with helm

```sh
helm repo add raczylo https://lukaszraczylo.github.io/helm-charts/
helm repo update raczylo
helm install jobs-manager raczylo/jobs-manager
```

### Prerequisites for local runs
- [go](https://golang.org/dl/) v1.16+
- [kustomize](https://sigs.k8s.io/kustomize/docs/INSTALL.md) v3.5.4+
- [docker](https://docs.docker.com/install/) v19.03.8+
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) v1.18.2+

### Jobs configuration

```yaml
apiVersion: jobsmanager.raczylo.com/v1beta1
kind: ManagedJob
metadata:
  labels:
  name: managedjob-sample
spec:
  retries: 3
  params:
    env:
      - name: "FOO"
        value: "bar"
      - name: "QUE"
        value: "pasa"

  # Job groups definitions
  groups:
    - name: "first-group"
      parallel: true
      params:
        env:
          - name: "FEE"
            value: "bee"
      jobs:
        - name: "first-job"
          image: "busybox"
          args:
            - "echo"
            - "Hello world!"
          params:
            env:
              - name: "POO"
                value: "paz"

        - name: "second-job"
          image: "busybox"
          args:
            - "sleep"
            - "10"
        - name: "second-half-job"
          image: "busybox"
          args:
            - "sleep"
            - "10"

    - name: "second-group"
      parallel: true
      jobs:
        - name: "third-job"
          image: "busybox"
          args:
            - "echo"
            - "Hello world!"
          parallel: true

        - name: "fourth-job"
          image: "busybox"
          args:
            - "sleep"
            - "10"
          parallel: false

    - name: "third-group"
      parallel: false
      jobs:
        - name: "fifth-job"
          image: "busybox"
          args:
            - "echo"
            - "Hello world!"
          parallel: true
```

### How does it look in practice?

```yaml
managedjob-sample
├── first-group
│   ├── first-job
│   ├── second-job
│   │   └── Depends on: managedjob-sample-first-group-first-job
│   └── second-half-job
│       ├── Depends on: managedjob-sample-first-group-first-job
│       └── Depends on: managedjob-sample-first-group-second-job
├── second-group
│   ├── third-job
│   └── fourth-job
│       └── Depends on: managedjob-sample-second-group-third-job
└── third-group
    ├── fifth-job
    ├── Depends on group: first-group
    └── Depends on group: second-group
```

If dependency exists on the group level - the group will not be executed until all of remaining groups have finished successfuly.
If dependency exists on the job level - the job will not be executed until all of remaining jobs have finished successfuly.
Remember that **ORDER matters**.

### Things to remember

Parameters **params** are always merged downwards to DRY your definitions.
In this case - result for the first job will look like this:

```yaml
    - jobs:
      - args:
        - echo
        - Hello world!
        compiledParams:
          env:
          - name: POO
            value: paz
          - name: FEE
            value: bee
          - name: FOO
            value: bar
          - name: QUE
            value: pasa
        image: busybox
        name: first-job
        parallel: false
        status: succeeded
```

### Available params

There's quite a lot of of flexibility with parameters. On every level where params are allowed, you can define:

```yaml
params:
  fromEnv:
    - configMapRef:
        name: "configmap-name"
      key: "key-name"
  env:
    - name: "FOO"
      value: "bar"
  volumes:
    - name: secrets-store-api
      csi:
        driver: secrets-store.csi.k8s.io
        readOnly: true
        volumeAttributes:
          secretProviderClass: api-secrets-provider
  volumeMount:
    - name: secrets-store-api
      mountPath: "/mnt/secrets-api"
      readOnly: true
  serviceAccount: "service-account-name"
  restartPolicy: "Never"
  imagePullSecrets:
    - "ghcr-token"
  imagePullPolicy:
    - "Always"
  labels:
    this/works: "true"
  annotations:
    this/works/aswell: "true"
```


### Kustomization and references

In case of any issues with `configmapGenerator` or `secretGenerator`, please add following to your `kustomization.yaml`:

```yaml
configurations:
  - crd-name-reference.yaml
```

Then you can create `crd-name-reference.yaml` file with following content:

```yaml
---
nameReference:
  - kind: 'ConfigMap'
    fieldSpecs:
      - kind: 'ManagedJob'
        path: 'spec/params/fromEnv[]/configMapRef/name'
      - kind: 'ManagedJob'
        path: 'spec/params/env[]/configMapRef/name'
```

This will instruct kustomize to replace all references to configmaps with their names if they are managed by generators.

### Running on the cluster

#### Manual installation
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=ghcr.io/lukaszraczylo/jobs-manager-operator:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=ghcr.io/lukaszraczylo/jobs-manager-operator:tag
```

#### Manually uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Manually undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```


### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

