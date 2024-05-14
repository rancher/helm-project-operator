FROM registry.suse.com/bci/golang:1.22 as builder
ARG helm-version=3.14.3
RUN zypper install -y gawk
RUN curl -LO "https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl"
RUN mv kubectl /usr/local/bin && chmod +x /usr/local/bin/kubectl
# TODO : debug
RUN which kubectl
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 |bash
# TODO : debug
RUN which helm
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.2
# TODO : start debug block
RUN which make
RUN which git
RUN which bash
# TODO : end debug block
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make ci

FROM registry.suse.com/bci/bci-micro:latest
RUN echo 'helmprojectoperator:x:1000:1000::/home/helmprojectoperator:/bin/bash' >> /etc/passwd && \
    echo 'helmprojectoperator:x:1000:' >> /etc/group && \
    mkdir /home/helmprojectoperator && \
    chown -R helmprojectoperator:helmprojectoperator /home/helmprojectoperator
COPY --from=builder /usr/src/app/bin/helm-project-operator /usr/bin/
USER helmprojectoperator
CMD ["helm-project-operator"]
