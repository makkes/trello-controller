# Trello Controller

A Kubernetes controller reflecting application workloads on a specific Trello board for simple insight into their health.

## Prerequisites

This controller makes use of [Flux](https://fluxcd.io) so you need to install Flux on your cluster and have the Flux CLI available locally.

## Installation and Usage

Installation consists of 2 steps: Installing the trello-controller and creating a Secret with Trello credentials.

1. Install the trello-controller

   ```sh
   flux create source oci trello-controller --url=oci://ghcr.io/makkes/manifests/trello-controller --tag=v0.0.2 --interval=10m
   flux create ks trello-controller --source=OCIRepository/trello-controller --path=./config/default --interval=1h

1. Create Secret with Trello credentials
   
   Obtain an API key as well as an API token from Trello as describe [in Trello's documentation](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/#authentication-and-authorization) and create a Kubernetes Secret from them:

   ```sh
   kubectl -n trello-system create secret generic trello-credentials --from-literal=api-key=YOUR_API_KEY --from-literal=api-token=YOUR_API_TOKEN --from-literal=list-id=YOUR_LIST_ID
   ```

As soon as the pod gets ready you should see all the Deployments on the cluster reflected on the given Trello list.

## Configuration

The type of resource that the controller watches and reflects on your Trello board is configurable. The default is to list all Deployments. In order to change the target type you'll need to patch the Kustomization to (1) configure the controller itself and (2) configure proper RBAC rules. Please see the [example Kustomization manifest](config/examples/watch-kustomizations.yaml) for how to do this.

## Releasing a new Version

1. edit `config/manager/kustomization.yaml` and set `newTag` to the new tag.
1. create the tag: `git tag -sam TAG TAG`
1. push the tag: `git push origin TAG`
1. build and push docker image and OCI artifact: `make release`
