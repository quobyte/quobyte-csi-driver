FROM ubuntu:22.04

RUN apt-get -y update && apt-get -y upgrade && apt-get install -y attr \
  && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

ADD quobyte-csi /bin

ENTRYPOINT ["/bin/quobyte-csi"]
