# Developer notes for release

1. Change the code and/or deployment files with the expected version.
   Push image using `../build container` command to (local) container
   hub (with pre tag) to test it.
2. Test changes with [E2E](e2e) tests
3. Deploy it into our production cluster (notify infra team to deploy) and
   let the new driver run for 1-2 two weeks.
4. If the steps 2 & 3 are passed, make release changes with `../build release`
   (it automatically updates and commits relevant files with release version,
   builds and pushes version images).
5. Make release on github.
6. Update [quobyte-k8s-helm](https://github.com/quobyte/quobyte-k8s-helm/tree/main/charts/quobyte-csi).
   (Copy over updated deployments and/or update values.yaml as required. For example, it could be behind
   couple of k8s major versions and update driver to suite those needs.)
