#!/bin/bash

DEFAULT_CONTAINER_URL_BASE="quay.io/quobyte/csi"
# change default with with CONTAINER_URL_BASE="<container-base-url>" ./build.sh
CONTAINER_URL_BASE="${CONTAINER_URL_BASE:-$DEFAULT_CONTAINER_URL_BASE}"

# buildx needs "load" for local testing (vs push to remote)
LOCAL_IMAGE="${LOCAL_IMAGE:-false}"
DISABLE_VERSION_VALIDATION="${DISABLE_VERSION_VALIDATION:-false}"

if [[ "$(dirname $0)" != '.' ]]; then
  echo "Executing build command in $(dirname $0)"
  cd "$(dirname $0)"
fi

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
  VERSION="$1"
  # must be of the form vX.Y.Z (ex, v1.8.3) -- any number of digits per component
  if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "FAILURE: '${VERSION}' is not a valid version string. Version must be of the form vX.Y.Z (ex, v1.8.3)"
    exit 1
  fi
}

container_build_and_push(){
  if [[ -z "${CONTAINER_URL_BASE}" ]]; then
    echo "FAILURE: container base url should not be empty"
    exit 1
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

# Tags HEAD with the release version and pushes just that tag to origin -- not "--tags" (which
# would push every local tag) and not the branch itself
create_and_push_release_tag(){
  local version="$1"
  validate_version "${version}"

  if [[ "$(git rev-parse --abbrev-ref HEAD)" != "master" ]]; then
    echo 'FAILURE: release can only be made on the master branch'
    exit 1
  fi
  if [[ -n "$(git status --porcelain)" ]]; then
    echo 'FAILURE: requires clean directory (no staged/unstaged/untracked files) in repo'
    exit 1
  fi

  git pull
  exit_if_failure "$?" 'Cannot pull latest master. Fix the reported issue and retry.'

  if [[ -n "$(git tag -l "${version}")" ]]; then
    echo "FAILURE: release version tag ${version} already exists on the local repo."
    exit 1
  fi
  if [[ -n "$(git ls-remote --tags origin "${version}")" ]]; then
    echo "FAILURE: release version tag ${version} already exists on the remote origin."
    exit 1
  fi

  git tag "${version}"
  exit_if_failure "$?" "Cannot tag release version ${version}. Fix the reported issue \
(you may need to run 'git tag -d ${version}' to undo it)."

  git push origin "${version}"
  exit_if_failure "$?" "Cannot push release tag ${version} to origin. Fix the reported issue \
(you may need to run 'git tag -d ${version}' and 'git push --delete origin ${version}' to undo it)."
}

print_post_release_instructions(){
  local version="$1"
  echo ''
  echo ''
  echo -e '\e[33mPlease go to https://github.com/quobyte/quobyte-csi/releases'
  echo -e "and make a release for the tag version ${version} with release notes\e[0m"
}

if [[ "$1" = '-h' || "$1" = '--help' ]]; then
  echo './build.sh                                 Builds the executable'
  echo './build.sh container <release-tag>         Builds and pushes container'
  echo './build.sh release <release-tag>           Builds and pushes container, then creates'
  echo '                                            and pushes the git release tag'
  echo "Example: ./build.sh [container] v0.2.0"
  echo '         ./build.sh release v1.8.4'
  exit 0
else
  echo 'Building executable'
  if [[ -f quobyte-csi ]]; then
    rm quobyte-csi
  fi
  echo "Generating //go:generate marked statements in source file"
  go generate ./...
  exit_if_failure "$?" "Failed generating required mocks for testing. Fix reported errors and retry."
  echo "Running tests..."
  go test -v ./...
  exit_if_failure "$?" "Failed go unit tests. Fix failing tests and retry command."

  if [[ $# -eq 0 ]]; then
    docker buildx build --platform linux/amd64,linux/arm64 .
    exit_if_failure "$?" "Building binary failed. Fix the reported errors and retry"
    exit 0
  fi

  if [[ "$1" == "container" ]]; then
    container_build_and_push "$2"
  elif [[ "$1" == "release" ]]; then
    container_build_and_push "$2"
    create_and_push_release_tag "$2"
    print_post_release_instructions "$2"
  fi
fi
