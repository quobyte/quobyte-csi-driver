FROM ubuntu:18.04

RUN apt-get -y update && apt-get install -y attr

ADD quobyte-csi /bin

ENTRYPOINT ["/bin/quobyte-csi"]
