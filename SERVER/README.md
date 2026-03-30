<!--BEGIN_BANNER_IMAGE-->

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="/.github/banner_dark.png">
  <source media="(prefers-color-scheme: light)" srcset="/.github/banner_light.png">
  <img style="width:100%;" alt="The HubLive icon, the name of the repository and some sample code in the background." src="https://raw.githubusercontent.com/hublive/hublive/main/.github/banner_light.png">
</picture>

<!--END_BANNER_IMAGE-->

# HubLive: Real-time video, audio and data for developers

[HubLive](https://HubLive.io) is an open source project that provides scalable, multi-user conferencing based on WebRTC.
It's designed to provide everything you need to build real-time video audio data capabilities in your applications.

HubLive's server is written in Go, using the awesome [Pion WebRTC](https://github.com/pion/webrtc) implementation.

[![GitHub stars](https://img.shields.io/github/stars/hublive/hublive?style=social&label=Star&maxAge=2592000)](https://github.com/hublive/hublive/stargazers/)
[![Slack community](https://img.shields.io/endpoint?url=https%3A%2F%2FHubLive.io%2Fbadges%2Fslack)](https://HubLive.io/join-slack)
[![Twitter Follow](https://img.shields.io/twitter/follow/HubLive)](https://twitter.com/HubLive)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/hublive/hublive)
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/hublive/hublive)](https://github.com/hublive/hublive/releases/latest)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/hublive/hublive/buildtest.yaml?branch=master)](https://github.com/hublive/hublive/actions/workflows/buildtest.yaml)
[![License](https://img.shields.io/github/license/hublive/hublive)](https://github.com/hublive/hublive/blob/master/LICENSE)

## Features

-   Scalable, distributed WebRTC SFU (Selective Forwarding Unit)
-   Modern, full-featured client SDKs
-   Built for production, supports JWT authentication
-   Robust networking and connectivity, UDP/TCP/TURN
-   Easy to deploy: single binary, Docker or Kubernetes
-   Advanced features including:
    -   [speaker detection](https://docs.HubLive.io/home/client/tracks/subscribe/#speaker-detection)
    -   [simulcast](https://docs.HubLive.io/home/client/tracks/publish/#video-simulcast)
    -   [end-to-end optimizations](https://blog.HubLive.io/HubLive-one-dot-zero/)
    -   [selective subscription](https://docs.HubLive.io/home/client/tracks/subscribe/#selective-subscription)
    -   [moderation APIs](https://docs.HubLive.io/home/server/managing-participants/)
    -   end-to-end encryption
    -   SVC codecs (VP9, AV1)
    -   [webhooks](https://docs.HubLive.io/home/server/webhooks/)
    -   [distributed and multi-region](https://docs.HubLive.io/home/self-hosting/distributed/)

## Documentation & Guides

https://docs.HubLive.io

## Live Demos

-   [HubLive Meet](https://meet.HubLive.io) ([source](https://github.com/HubLive-examples/meet))
-   [Spatial Audio](https://spatial-audio-demo.HubLive.io/) ([source](https://github.com/HubLive-examples/spatial-audio))
-   Livestreaming from OBS Studio ([source](https://github.com/HubLive-examples/livestream))
-   [AI voice assistant using ChatGPT](https://HubLive.io/kitt) ([source](https://github.com/HubLive-examples/kitt))

## Ecosystem

-   [Agents](https://github.com/HubLive/agents): build real-time multimodal AI applications with programmable backend participants
-   [Egress](https://github.com/HubLive/egress): record or multi-stream rooms and export individual tracks
-   [Ingress](https://github.com/HubLive/ingress): ingest streams from external sources like RTMP, WHIP, HLS, or OBS Studio

## SDKs & Tools

### Client SDKs

Client SDKs enable your frontend to include interactive, multi-user experiences.

<table>
  <tr>
    <th>Language</th>
    <th>Repo</th>
    <th>
        <a href="https://docs.HubLive.io/home/client/events/#declarative-ui" target="_blank" rel="noopener noreferrer">Declarative UI</a>
    </th>
    <th>Links</th>
  </tr>
  <!-- BEGIN Template
  <tr>
    <td>Language</td>
    <td>
      <a href="" target="_blank" rel="noopener noreferrer"></a>
    </td>
    <td></td>
    <td></td>
  </tr>
  END -->
  <!-- JavaScript -->
  <tr>
    <td>JavaScript (TypeScript)</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-js" target="_blank" rel="noopener noreferrer">client-sdk-js</a>
    </td>
    <td>
      <a href="https://github.com/hublive/hublive-react" target="_blank" rel="noopener noreferrer">React</a>
    </td>
    <td>
      <a href="https://docs.HubLive.io/client-sdk-js/" target="_blank" rel="noopener noreferrer">docs</a>
      |
      <a href="https://github.com/HubLive/client-sdk-js/tree/main/example" target="_blank" rel="noopener noreferrer">JS example</a>
      |
      <a href="https://github.com/HubLive/client-sdk-js/tree/main/example" target="_blank" rel="noopener noreferrer">React example</a>
    </td>
  </tr>
  <!-- Swift -->
  <tr>
    <td>Swift (iOS / MacOS)</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-swift" target="_blank" rel="noopener noreferrer">client-sdk-swift</a>
    </td>
    <td>Swift UI</td>
    <td>
      <a href="https://docs.HubLive.io/client-sdk-swift/" target="_blank" rel="noopener noreferrer">docs</a>
      |
      <a href="https://github.com/HubLive/client-example-swift" target="_blank" rel="noopener noreferrer">example</a>
    </td>
  </tr>
  <!-- Kotlin -->
  <tr>
    <td>Kotlin (Android)</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-android" target="_blank" rel="noopener noreferrer">client-sdk-android</a>
    </td>
    <td>Compose</td>
    <td>
      <a href="https://docs.HubLive.io/client-sdk-android/index.html" target="_blank" rel="noopener noreferrer">docs</a>
      |
      <a href="https://github.com/HubLive/client-sdk-android/tree/main/sample-app/src/main/java/io/HubLive/android/sample" target="_blank" rel="noopener noreferrer">example</a>
      |
      <a href="https://github.com/HubLive/client-sdk-android/tree/main/sample-app-compose/src/main/java/io/HubLive/android/composesample" target="_blank" rel="noopener noreferrer">Compose example</a>
    </td>
  </tr>
<!-- Flutter -->
  <tr>
    <td>Flutter (all platforms)</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-flutter" target="_blank" rel="noopener noreferrer">client-sdk-flutter</a>
    </td>
    <td>native</td>
    <td>
      <a href="https://docs.HubLive.io/client-sdk-flutter/" target="_blank" rel="noopener noreferrer">docs</a>
      |
      <a href="https://github.com/HubLive/client-sdk-flutter/tree/main/example" target="_blank" rel="noopener noreferrer">example</a>
    </td>
  </tr>
  <!-- Unity -->
  <tr>
    <td>Unity WebGL</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-unity-web" target="_blank" rel="noopener noreferrer">client-sdk-unity-web</a>
    </td>
    <td></td>
    <td>
      <a href="https://HubLive.github.io/client-sdk-unity-web/" target="_blank" rel="noopener noreferrer">docs</a>
    </td>
  </tr>
  <!-- React Native -->
  <tr>
    <td>React Native (beta)</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-react-native" target="_blank" rel="noopener noreferrer">client-sdk-react-native</a>
    </td>
    <td>native</td>
    <td></td>
  </tr>
  <!-- Rust -->
  <tr>
    <td>Rust</td>
    <td>
      <a href="https://github.com/HubLive/client-sdk-rust" target="_blank" rel="noopener noreferrer">client-sdk-rust</a>
    </td>
    <td></td>
    <td></td>
  </tr>
</table>

### Server SDKs

Server SDKs enable your backend to generate [access tokens](https://docs.HubLive.io/home/get-started/authentication/),
call [server APIs](https://docs.HubLive.io/reference/server/server-apis/), and
receive [webhooks](https://docs.HubLive.io/home/server/webhooks/). In addition, the Go SDK includes client capabilities,
enabling you to build automations that behave like end-users.

| Language                | Repo                                                                                    | Docs                                                        |
| :---------------------- | :-------------------------------------------------------------------------------------- | :---------------------------------------------------------- |
| Go                      | [server-sdk-go](https://github.com/HubLive/server-sdk-go)                               | [docs](https://pkg.go.dev/github.com/HubLive/server-sdk-go) |
| JavaScript (TypeScript) | [server-sdk-js](https://github.com/HubLive/server-sdk-js)                               | [docs](https://docs.HubLive.io/server-sdk-js/)              |
| Ruby                    | [server-sdk-ruby](https://github.com/HubLive/server-sdk-ruby)                           |                                                             |
| Java (Kotlin)           | [server-sdk-kotlin](https://github.com/HubLive/server-sdk-kotlin)                       |                                                             |
| Python (community)      | [python-sdks](https://github.com/HubLive/python-sdks)                                   |                                                             |
| PHP (community)         | [agence104/hublive-server-sdk-php](https://github.com/agence104/hublive-server-sdk-php) |                                                             |

### Tools

-   [CLI](https://github.com/hublive/hublive-cli) - command line interface & load tester
-   [Docker image](https://hub.docker.com/r/HubLive/hublive-server)
-   [Helm charts](https://github.com/hublive/hublive-helm)

## Install

> [!TIP]
> We recommend installing [HubLive CLI](https://github.com/hublive/hublive-cli) along with the server. It lets you access
> server APIs, create tokens, and generate test traffic.

The following will install HubLive's media server:

### MacOS

```shell
brew install HubLive
```

### Linux

```shell
curl -sSL https://get.HubLive.io | bash
```

### Windows

Download the [latest release here](https://github.com/hublive/hublive/releases/latest)

## Getting Started

### Starting HubLive

Start HubLive in development mode by running `hublive-server --dev`. It'll use a placeholder API key/secret pair.

```
API Key: devkey
API Secret: secret
```

To customize your setup for production, refer to our [deployment docs](https://docs.HubLive.io/deploy/)

### Creating access token

A user connecting to a HubLive room requires an [access token](https://docs.HubLive.io/home/get-started/authentication/#creating-a-token). Access
tokens (JWT) encode the user's identity and the room permissions they've been granted. You can generate a token with our
CLI:

```shell
lk token create \
    --api-key devkey --api-secret secret \
    --join --room my-first-room --identity user1 \
    --valid-for 24h
```

### Test with example app

Head over to our [example app](https://example.HubLive.io) and enter a generated token to connect to your HubLive
server. This app is built with our [React SDK](https://github.com/hublive/hublive-react).

Once connected, your video and audio are now being published to your new HubLive instance!

### Simulating a test publisher

```shell
lk room join \
    --url ws://localhost:7880 \
    --api-key devkey --api-secret secret \
    --identity bot-user1 \
    --publish-demo \
    my-first-room
```

This command publishes a looped demo video to a room. Due to how the video clip was encoded (keyframes every 3s),
there's a slight delay before the browser has sufficient data to begin rendering frames. This is an artifact of the
simulation.

## Deployment

### Use HubLive Cloud

HubLive Cloud is the fastest and most reliable way to run HubLive. Every project gets free monthly bandwidth and
transcoding credits.

Sign up for [HubLive Cloud](https://cloud.HubLive.io/).

### Self-host

Read our [deployment docs](https://docs.HubLive.io/transport/self-hosting/) for more information.

## Building from source

Pre-requisites:

-   Go 1.23+ is installed
-   GOPATH/bin is in your PATH

Then run

```shell
git clone https://github.com/hublive/hublive
cd HubLive
./bootstrap.sh
mage
```

## Contributing

We welcome your contributions toward improving HubLive! Please join us
[on Slack](http://HubLive.io/join-slack) to discuss your ideas and/or PRs.

## License

HubLive server is licensed under Apache License v2.0.

<!--BEGIN_REPO_NAV-->
<br/><table>
<thead><tr><th colspan="2">HubLive Ecosystem</th></tr></thead>
<tbody>
<tr><td>Agents SDKs</td><td><a href="https://github.com/HubLive/agents">Python</a> · <a href="https://github.com/HubLive/agents-js">Node.js</a></td></tr><tr></tr>
<tr><td>HubLive SDKs</td><td><a href="https://github.com/HubLive/client-sdk-js">Browser</a> · <a href="https://github.com/HubLive/client-sdk-swift">Swift</a> · <a href="https://github.com/HubLive/client-sdk-android">Android</a> · <a href="https://github.com/HubLive/client-sdk-flutter">Flutter</a> · <a href="https://github.com/HubLive/client-sdk-react-native">React Native</a> · <a href="https://github.com/HubLive/rust-sdks">Rust</a> · <a href="https://github.com/HubLive/node-sdks">Node.js</a> · <a href="https://github.com/HubLive/python-sdks">Python</a> · <a href="https://github.com/HubLive/client-sdk-unity">Unity</a> · <a href="https://github.com/HubLive/client-sdk-unity-web">Unity (WebGL)</a> · <a href="https://github.com/HubLive/client-sdk-esp32">ESP32</a> · <a href="https://github.com/HubLive/client-sdk-cpp">C++</a></td></tr><tr></tr>
<tr><td>Starter Apps</td><td><a href="https://github.com/HubLive-examples/agent-starter-python">Python Agent</a> · <a href="https://github.com/HubLive-examples/agent-starter-node">TypeScript Agent</a> · <a href="https://github.com/HubLive-examples/agent-starter-react">React App</a> · <a href="https://github.com/HubLive-examples/agent-starter-swift">SwiftUI App</a> · <a href="https://github.com/HubLive-examples/agent-starter-android">Android App</a> · <a href="https://github.com/HubLive-examples/agent-starter-flutter">Flutter App</a> · <a href="https://github.com/HubLive-examples/agent-starter-react-native">React Native App</a> · <a href="https://github.com/HubLive-examples/agent-starter-embed">Web Embed</a></td></tr><tr></tr>
<tr><td>UI Components</td><td><a href="https://github.com/HubLive/components-js">React</a> · <a href="https://github.com/HubLive/components-android">Android Compose</a> · <a href="https://github.com/HubLive/components-swift">SwiftUI</a> · <a href="https://github.com/HubLive/components-flutter">Flutter</a></td></tr><tr></tr>
<tr><td>Server APIs</td><td><a href="https://github.com/HubLive/node-sdks">Node.js</a> · <a href="https://github.com/HubLive/server-sdk-go">Golang</a> · <a href="https://github.com/HubLive/server-sdk-ruby">Ruby</a> · <a href="https://github.com/HubLive/server-sdk-kotlin">Java/Kotlin</a> · <a href="https://github.com/HubLive/python-sdks">Python</a> · <a href="https://github.com/HubLive/rust-sdks">Rust</a> · <a href="https://github.com/agence104/hublive-server-sdk-php">PHP (community)</a> · <a href="https://github.com/pabloFuente/hublive-server-sdk-dotnet">.NET (community)</a></td></tr><tr></tr>
<tr><td>Resources</td><td><a href="https://docs.HubLive.io">Docs</a> · <a href="https://docs.HubLive.io/mcp">Docs MCP Server</a> · <a href="https://github.com/hublive/hublive-cli">CLI</a> · <a href="https://cloud.HubLive.io">HubLive Cloud</a></td></tr><tr></tr>
<tr><td>HubLive Server OSS</td><td><b>HubLive server</b> · <a href="https://github.com/HubLive/egress">Egress</a> · <a href="https://github.com/HubLive/ingress">Ingress</a> · <a href="https://github.com/HubLive/sip">SIP</a></td></tr><tr></tr>
<tr><td>Community</td><td><a href="https://community.HubLive.io">Developer Community</a> · <a href="https://HubLive.io/join-slack">Slack</a> · <a href="https://x.com/HubLive">X</a> · <a href="https://www.youtube.com/@HubLive_io">YouTube</a></td></tr>
</tbody>
</table>
<!--END_REPO_NAV-->
