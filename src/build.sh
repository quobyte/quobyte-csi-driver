
#!/bin/bash

DEFAULT_CONTAINER_URL_BASE="quay.io/quobyte/csi"
# change default with with CONTAINER_URL_BASE="<container-base-url>" ./build
CONTAINER_URL_BASE="${CONTAINER_URL_BASE:-$DEFAULT_CONTAINER_URL_BASE}"
# https://helm.sh/docs/topics/chart_repository/#github-pages-example
# Quobyte CSI charts are hosted as github pages. Artifacthub.io uses this
# location to grab the deployable charts from docs/index.yaml

# buildx needs "load" for local testing (vs push to remote)
LOCAL_IMAGE="${LOCAL_IMAGE:-false}"
DISABLE_VERSION_VALIDATION="${DISABLE_VERSION_VALIDATION:-false}"

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

# Enable docker containerd storage backend to build multi-arch images and load them for tests
docker info -f '{{ .DriverStatus }}' | grep "io.containerd.snapshotter.v1" > /dev/null 2>&1
exit_if_failure "$?" "Enable docker containerd storage backend and retry"
(docker buildx use multi-builder || docker buildx create --name multi-builder --use) > /dev/null 2>&1
exit_if_failure "$?" "Cannot use docker multi-builder to build multi-arch image"

validate_version() {
  if [[ ${DISABLE_VERSION_VALIDATION} = 'true' ]]; then
    return
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
}

container_build_and_push(){
  if [[ -z "${CONTAINER_URL_BASE}" ]]; then
    echo "FAILURE: container base url should not be empty"
  fi
  validate_version $1
  VERSION=$1
  IMAGE="${CONTAINER_URL_BASE}:${VERSION}"
  image_store_option="--push"
  if [[ "${LOCAL_IMAGE}" = 'true' ]]; then
    image_store_option="--load"
  fi
  docker buildx build --platform linux/amd64,linux/arm64 -t "$IMAGE" ${image_store_option} .
  push_succeeded="$?"
  if [[ ${push_succeeded} -ne 0 ]]; then
    echo "FAILURE: container image ${IMAGE} cannot be pushed"
    echo 'Please fix the reported issues and retry'
    exit 1
  fi
}

print_post_release_instructions(){
  echo ''
  echo ''
  echo -e '\e[33mPlease go to https://github.com/quobyte/quobyte-csi/releases'
  echo -e "and make a release for the tag version ${VERSION} with release notes\e[0m"
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
  echo "Generating //go:generate marked statements in source file"
  go generate ./...
  exit_if_failure "$?" "Failed generating required mocks for testing. Fix reported errors and retry"
  echo "Running tests..."
  go test -v ./...
  exit_if_failure "$?" "Failed go unit tests. Fix failing tests and retry command."

  if [[ $# -eq 0 ]]; then
    docker buildx build --platform linux/amd64,linux/arm64 .
    exit_if_failure "$?" "Building binary failed. Fix the reported errors and retry"
    exit 0
  fi

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
    if [[ ! -z "$(git tag -l ${VERSION})" ]]; then
      exit_if_failure "1" "Release version tag already exists on local repo."
    fi
    container_build_and_push $2
    if [[ ! -z "$(git ls-remote --tags origin ${VERSION})" ]]; then
      exit_if_failure "1" "Release version tag already exists on the remote origin."
    fi
    # Assumption is, at this point we do not have any modified files except
    # those modified by the script 
    git add -A
    git commit -m "Release version ${VERSION} by ./build release command"
    # TODO(venkat) - check if tag already exists
    git tag "${VERSION}"
    exit_if_failure "$1" "Cannot tag release version. Fix the reported issue (you may need to reset head to undo "Release commit")"
    git push origin master --tags
    print_post_release_instructions
  fi
fi
