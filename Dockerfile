# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.26.2-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY pkg ./pkg

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/csiplugin ./cmd/csiplugin

FROM registry.k8s.io/build-image/debian-base:bookworm-v1.0.5

RUN apt-get update && apt-get upgrade -y && apt-mark unhold libcap2 && clean-install util-linux e2fsprogs mount ca-certificates udev xfsprogs btrfs-progs open-iscsi && rm -rf /var/lib/apt/lists/*

CMD ["service", "iscsid", "start"]
COPY --from=builder /out/csiplugin /csiplugin

ENTRYPOINT ["/csiplugin"]
