FROM registry.suse.com/bci/golang:1.22 AS helm
RUN zypper -n install git
RUN git -C / clone --branch release-v3.9.0 --depth=1 https://github.com/rancher/helm
RUN make -C /helm

FROM registry.suse.com/bci/golang:1.22 AS builder
WORKDIR /usr/src/app
ENV YQ_VERSION=v4.25.1
RUN zypper -n install git curl wget make
RUN export ARCH=$(go env GOHOSTARCH)
RUN wget https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64 -O /usr/bin/yq && chmod +x /usr/bin/yq;
COPY --from=helm ./helm/bin/helm /usr/local/bin/
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build-chart
RUN make build

FROM registry.suse.com/bci/bci-micro:15.5
RUN echo 'helmprojectoperator:x:1000:1000::/home/helmprojectoperator:/bin/bash' >> /etc/passwd && \
    echo 'helmprojectoperator:x:1000:' >> /etc/group && \
    mkdir /home/helmprojectoperator && \
    chown -R helmprojectoperator:helmprojectoperator /home/helmprojectoperator
COPY --from=builder /usr/src/app/bin/helm-project-operator /usr/bin/
USER helmprojectoperator
CMD ["helm-project-operator"]
