# Trello Controller

This repository hosts a Kubernetes controller reflecting application workloads on a specific Trello board for simple insight.

## Prerequisites

This controller makes use of [Flux](https://fluxcd.io) so you need to install Flux on your cluster and have the Flux CLI available locally.

## Installation and Usage

Installation consists of 2 steps: Installing the trello-controller and creating a Secret with Trello credentials.

1. Install the trello-controller

   ```sh
   flux create source oci trello-controller --url=oci://ghcr.io/makkes/manifests/trello-controller --tag=v0.0.2 --interval=10m
   flux create ks trello-controller --source=OCIRepository/trello-controller --path=./config/default

1. Create Secret with Trello credentials
   
   Obtain an API key as well as an API token from Trello as describe [in Trello's documentation](https://developer.atlassian.com/cloud/trello/guides/rest-api/api-introduction/#authentication-and-authorization) and create a Kubernetes Secret from them:

   ```sh
   kubectl -n trello-system create secret generic trello-credentials --from-literal=api-key=YOUR_API_KEY --from-literal=api-token=YOUR_API_TOKEN --from-literal=list-id=YOUR_LIST_ID
   ```

As soon as the pod gets ready you should see all the Kustomizations on the cluster reflected on the given Trello list.
