# Kubernetes Jobs Manager Operator

## Description
This operator is responsible for managing the lifecycle of complicated workflows which consist of multiple jobs and making their management easy, without need for dozens of yaml files and doing magic with ordering.

## Getting Started

### Prerequisites
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
  # Globally defined parameters and environment variables
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
      # Group will run in parallel with other defined groups
      parallel: true
      # Group specific parameters
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
          parallel: true
```

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

