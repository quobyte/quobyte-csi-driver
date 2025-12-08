FROM alpine AS build

RUN mkdir /final_root/; \
    cd /final_root; \
    mkdir -p var tmp run bin; \
    ln -s var/run run; \
    chmod -R 755 var tmp run bin
COPY quobyte-csi /final_root/bin

FROM scratch
COPY --from=build /final_root/ /
ENTRYPOINT ["/bin/quobyte-csi"]
