FROM gcr.io/distroless/static-debian13
ARG TARGETPLATFORM

COPY build/config.yaml.example /config.yaml.example
COPY LICENSE /LICENSE
COPY README.md /README.md
COPY $TARGETPLATFORM/nginx-asg-sync /nginx-asg-sync

CMD ["/nginx-asg-sync"]
