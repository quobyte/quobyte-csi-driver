# Developer notes for release

1. Change the code and/or deployment files with the expected version and
   push image to (local) container hub (with pre tag) to test it.
2. Test changes with [E2E](e2e) tests
3. Change the [Chart Version](../quobyte-csi-driver/Chart.yaml) of the CSI driver
   with expected release.
4. Build release container using `<PROJECT_root>/build <chart_version_from_step_2>`
5. Deploy it into our production cluster (notify infra team to deploy) and
   let the new driver run for 1-2 two weeks.
6. Make release on github
7. Update [quobyte-k8s-helm](https://github.com/quobyte/quobyte-k8s-helm/tree/main/charts/quobyte-csi).
   (Copy over updated deployments and/or update values.yaml as required)
