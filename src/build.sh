
#!/bin/bash
DEFAULT_CONTAINER_URL_BASE="quay.io/quobyte/csi"
# change default with with CONTAINER_URL_BASE="<container-base-url>" ./build
CONTAINER_URL_BASE="${CONTAINER_URL_BASE:-$DEFAULT_CONTAINER_URL_BASE}"
# https://helm.sh/docs/topics/chart_repository/#github-pages-example
# Quobyte CSI charts are hosted as github pages. Artifacthub.io uses this
# location to grab the deployable charts from docs/index.yaml

if [[ "$(dirname $0)" != '.' ]]; then
  echo "Executing build command in $(dirname $0)"
  cd "$(dirname $0)"
fi

CHART_PACKAGE_DIR="../helm" 
CHART_DIR="../csi-driver-templates"

# arg1 - command exit status, arg2 - error message to be printed
exit_if_failure() {
  if [[ $1 -ne 0 ]]; then
    echo "$2"
    exit 1
  fi
}

container_build_and_push(){
  if [[ -z "${CONTAINER_URL_BASE}" ]]; then
    echo "FAILURE: container base url should not be empty"
  fi
  VERSION=$1
  if [[ -z "${VERSION}" || "{$VERSION}" == *\ * ]]; then
    echo "FAILURE: ${VERSION} is not a valid version string. Version must not be empty or should not contain any spaces"
    exit 1
  fi
  # make sure version starts with v and has exactly 6 chars (vX.Y.Z)
  if [[ "$VERSION" != v.* && "${#VERSION}" -ne 6 ]]; then
    echo "version must start be of the form vX.Y.Z (ex, v1.8.3)"
    exit 1
  fi
  IMAGE="${CONTAINER_URL_BASE}:${VERSION}"
  echo "Building docker image and pushing it to ${IMAGE}"
  sudo docker build -t quobyte-csi -f Dockerfile .
  sudo docker run -it quobyte-csi
  CSI_RUN_ID="$(sudo docker ps -l | grep 'quobyte-csi' | awk '{print $1}')"
  echo "Pushing $CSI_RUN_ID to ${IMAGE}"
  sudo docker commit "$CSI_RUN_ID" "$IMAGE"
  sudo docker push "$IMAGE"
  push_succeeded="$?"
  if [[ ${push_succeeded} -ne 0 ]]; then
    echo "FAILURE: container image ${IMAGE} cannot be pushed"
    echo 'Please fix the reported issues and retry'
    exit 1
  fi
}

rebase_charts_on_master(){
  echo 'updating master with version files...'
  git push origin master
  git checkout charts
  echo 'rebasing charts on current master...'
  git rebase master
  echo 'updating charts with rebased version...'
  git push origin charts
  echo 'switching back to master...'
  git checkout master
}

print_post_release_instructions(){
  echo ''
  echo ''
  echo 'Please go to https://github.com/quobyte/quobyte-csi/releases'
  echo "and make a release for the tag version ${VERSION} with release notes"
}

build_helm_package(){
  helm package -d "${CHART_PACKAGE_DIR}" "${CHART_DIR}" 
  helm repo index "${CHART_PACKAGE_DIR}"
}

update_files_with_version(){
  sed -i "s|appVersion:.*|appVersion: \"${VERSION}\"|g" "${CHART_DIR}/Chart.yaml"
  sed -i "s|version:.*|version: \"${CHART_VERSION}\"|g" "${CHART_DIR}/Chart.yaml"
  sed -i "s|.*csiProvisionerVersion:.*|    csiProvisionerVersion: \"${VERSION}\"|g" "${CHART_DIR}/values.yaml"
  sed -i "s|.*csiImage:.*|    csiImage: \"${CONTAINER_URL_BASE}:${VERSION}\"|g" "${CHART_DIR}/values.yaml"
  sed -i "s|- --driver_version=.*|- --driver_version=${VERSION}|g" "${CHART_DIR}/tests/__snapshot__/csi_driver_test.yaml.snap"
  sed -i "s|image: quay.io/quobyte/csi:.*|image: quay.io/quobyte/csi:${VERSION}|g" "${CHART_DIR}/tests/__snapshot__/csi_driver_test.yaml.snap"
}

if [[ "$1" = '-h' || "$1" = '--help' ]]; then
  echo './build                                                 Builds the executable'
  echo './build container <release-tag>"                        Builds pre and pushes container'
  echo './build release "<release-tag>" "<chart-version>"       Builds the executable, docker image and'
  echo '                                         pushes the container and creates a helm chart'
  echo '                                         for the release'
  echo "Example: ./build [container] v0.2.0"
  echo "         ./build release v1.8.4 1.9.4"
  exit 0
else
  echo 'Building executable'
  if [[ -f quobyte-csi ]]; then
    rm quobyte-csi
  fi
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o quobyte-csi ./cmd/main.go
  exit_if_failure "$?" "Building binary failed. Fix the reported errors and retry"
  echo "Generating //go:generate marked statemets in source file"
  go generate ./...
  exit_if_failure "$?" "Failed generating required mocks for testing. Fix reported errors and retry"
  echo "Running tests..."
  go test ./...
  exit_if_failure "$?" "Failed tests. Fix failing tests and retry command."

  if [[ "$1" == "container" ]]; then
    container_build_and_push $2
  elif [[ "$1" == "release" ]]; then
    if [[ $(git rev-parse --abbrev-ref HEAD) != "master" ]]; then
      echo 'FAILURE: release can only be made on master branch'
      exit 1
    fi
    if [[ ! -z "$(git status --porcelain)" ]]; then
      echo 'Requires clean directory (no stage/unstaged/untracked files) in repo'
      exit 1
    fi
    git pull
    container_build_and_push $2
    if [[ $(command -v helm &> /dev/null; echo "$?" ) -eq 1 ]]; then 
       (cd /tmp && curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 \
          && chmod 700 get_helm.sh && ./get_helm.sh)
    fi
    echo "Updating chart, and CSI driver files with release version ${VERSION}"
    if [[ -z "$3" ]]; then
      echo "Requires helm chart version"
      exit 1
    fi
    CHART_VERSION="$3"
    if [[ "$CHART_VERION" = v* ]]; then
    echo "chart version should be semversion (X.Y.Z) - v prefix is allowed"
    exit 1
    fi
    update_files_with_version
    build_helm_package
    echo "Adding packaged chart to docs"
    git add ${CHART_PACKAGE_DIR}/index.yaml
    git add ${CHART_PACKAGE_DIR}/*.tgz
    # Assumption is, at this point we do not have any modified files except
    # those modified by the script 
    git add -A
    git commit -m "Release version ${VERSION} by ./build release command"
    git tag "${VERSION}"
    # update chart index fiel for Artifacthub to get new update
    rebase_charts_on_master
    git push origin master --tags
    print_post_release_instructions
  fi
fi
