FROM ubuntu:20.04

RUN apt-get -y update && apt-get -y upgrade && apt-get install -y attr

ADD quobyte-csi /bin

ENTRYPOINT ["/bin/quobyte-csi"]
