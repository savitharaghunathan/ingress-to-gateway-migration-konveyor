# Migrating Go source code from Ingress-NGINX to Gateway API using Konveyor

- [Migrating Go source code from Ingress-NGINX to Gateway API using Konveyor](#migrating-go-source-code-from-ingress-nginx-to-gateway-api-using-konveyor)
  - [Goal](#goal)
  - [Overview](#overview)
  - [Prerequisites](#prerequisites)
  - [Step 1: Setup](#step-1-setup)
    - [Clone the Repository](#clone-the-repository)
    - [Install the Konveyor Extensions](#install-the-konveyor-extensions)
  - [Step 2: Configure Analysis](#step-2-configure-analysis)
    - [2.1 Open the Project](#21-open-the-project)
    - [2.2 Configure Analysis Profile](#22-configure-analysis-profile)
    - [2.3 Select Custom Rules](#23-select-custom-rules)
    - [2.4 Set Source and Target Labels](#24-set-source-and-target-labels)
  - [Step 3: Run Analysis](#step-3-run-analysis)
    - [3.1 Start the Server](#31-start-the-server)
    - [3.2 Run Analysis](#32-run-analysis)
    - [3.3 Analysis Results](#33-analysis-results)
  - [Step 4: Review Violations](#step-4-review-violations)
    - [Type Reference Violations](#type-reference-violations)
    - [Client-go API Violations](#client-go-api-violations)
    - [Annotation Violations](#annotation-violations)
  - [Step 5: Configure KAI (GenAI)](#step-5-configure-kai-genai)
  - [Step 6: Generate Fixes with KAI](#step-6-generate-fixes-with-kai)
    - [6.1 Fix Type References and Client-go API Calls](#61-fix-type-references-and-client-go-api-calls)
    - [6.2 Fix Annotation Violations](#62-fix-annotation-violations)
  - [Step 7: Verify the Migration](#step-7-verify-the-migration)
  - [What the Rules Cover](#what-the-rules-cover)

## Demo

[![Migrating Go source code from Ingress-NGINX to Gateway API using Konveyor](https://img.youtube.com/vi/qNdqfI7wgfM/0.jpg)](https://youtu.be/qNdqfI7wgfM)

## Goal

Migrate a Go application that programmatically creates Kubernetes Ingress resources using `k8s.io/api/networking/v1` and `client-go` to the [Gateway API](https://gateway-api.sigs.k8s.io/). We will use custom rules with Konveyor to analyze the application and KAI to generate migration fixes.

## Overview

[Ingress-NGINX](https://github.com/kubernetes/ingress-nginx) is being retired in March 2026. The Kubernetes project recommends migrating to the Gateway API.

Tools like [`ingress2gateway`](https://github.com/kubernetes-sigs/ingress2gateway) can convert Ingress YAML manifests to Gateway API resources, but they cannot help with **Go programs that build Ingress objects in code**. These programs construct Ingress resources at runtime using `k8s.io/api/networking/v1` types, set nginx annotations as string literals, and call the Kubernetes API via `client-go`. The Ingress resources only exist at runtime, not as static YAML.

Konveyor addresses this gap:

- **Discovery** — the Konveyor Go extension scans Go source to find every reference to Ingress types, client-go API calls, and nginx annotation strings
- **Migration guidance** — each rule produces a violation with concrete before/after Go code examples showing the Gateway API equivalent
- **KAI fixes** — KAI uses the rule messages to generate context-aware Gateway API code via LLM

### Custom Rules for Ingress-NGINX Migration

This scenario uses 26 custom rules in `go-rules.yaml` that detect Ingress-NGINX patterns in Go source code. The rules use two providers:

#### `go.referenced` rules (8 rules)

Detect usage of `networkingv1` types via the Go LSP:

```yaml
- ruleID: ingress-nginx-go-ref-00001
  description: "Go code references networkingv1.Ingress type"
  category: mandatory
  effort: 3
  when:
    go.referenced:
      pattern: '\bIngress\b'
  message: |
    This code references `networkingv1.Ingress`. Migrate to
    `gatewayv1.HTTPRoute`.
    ...
```

#### `builtin.filecontent` rules (18 rules)

Detect client-go API calls and nginx annotation string literals:

```yaml
- ruleID: ingress-nginx-go-ref-00028
  description: "Go code sets nginx proxy-read-timeout annotation"
  category: mandatory
  effort: 1
  when:
    builtin.filecontent:
      pattern: 'nginx\.ingress\.kubernetes\.io/proxy-(read|send|connect)-timeout'
      filePattern: "*.go"
  message: |
    Replace proxy timeout annotations with HTTPRoute timeouts:
      rule := gatewayv1.HTTPRouteRule{
          Timeouts: &gatewayv1.HTTPRouteTimeouts{
              Request:        ptr(gatewayv1.Duration("60s")),
              BackendRequest: ptr(gatewayv1.Duration("30s")),
          },
      }
```

## Prerequisites

- [VSCode](https://code.visualstudio.com/download)
- [Git](https://git-scm.com/downloads)
- [Go toolchain](https://go.dev/dl/) (1.22+)
- AI credentials (OpenAI, Amazon Bedrock, Ollama, etc.)

Additionally, you will need to have the Konveyor IDE plugin installed in VSCode. Download the latest from [here](https://github.com/konveyor/editor-extensions/releases).

## Step 1: Setup

### Clone the Repository

```shell
git clone https://github.com/savitharaghunathan/ingress-to-gateway-migration-konveyor.git
cd ingress-to-gateway-migration-konveyor
```

### Install the Konveyor Extensions

Open VSCode and go to the Extensions view (`Cmd+Shift+X` on macOS, `Ctrl+Shift+X` on Linux/Windows).

Install two extensions:

1. **Konveyor** (`konveyor.konveyor-core`) — the core extension that provides static analysis, rule management, and the KAI integration
2. **Konveyor Go** (`konveyor.konveyor-go`) — adds the Go analysis provider, enabling `go.referenced` and `go.dependency` rule conditions

The Go language extension (`golang.go`) is installed automatically as a dependency of Konveyor Go.

## Step 2: Configure Analysis

### 2.1 Open the Project

Navigate to File > Open in VSCode and open the `go-app-v2/` folder as the workspace root. The repository structure is:

```
ingress-to-gateway-migration-konveyor/
├── go-app-v2/          ← open this folder in VSCode
│   ├── main.go
│   ├── go.mod
│   └── go.sum
├── rules/              ← custom rules (referenced in step 2.3)
│   ├── ruleset.yaml
│   └── go-rules.yaml
└── README.md
```

The app is a single Go file (`main.go`) — a CLI tool that provisions Kubernetes Ingress resources for tenant applications using nginx-ingress. It uses `k8s.io/api v0.31.0` and `k8s.io/client-go v0.31.0`.

### 2.2 Configure Analysis Profile

Click the Konveyor extension icon in the sidebar, then click the settings icon to configure the analysis profile.

### 2.3 Select Custom Rules

In the configuration dialog, click **Set Rules** and navigate to the `rules/` directory in the cloned repository (one level up from `go-app-v2/`).

The rules directory contains:

| File | Description |
|---|---|
| `ruleset.yaml` | Ruleset metadata and labels (`konveyor.io/source=go`, `konveyor.io/target=go`) |
| `go-rules.yaml` | 26 rules — 8 `go.referenced` rules for type detection, 18 `builtin.filecontent` rules for API calls and annotations |

### 2.4 Set Source and Target Labels

Set **Source** to `go` and **Target** to `go`. The analyzer filters rules by these labels — rules without matching labels are excluded from analysis.

## Step 3: Run Analysis

### 3.1 Start the Server

Click **Start** in the Konveyor Analysis view to launch the analyzer and RPC server.

### 3.2 Run Analysis

Once the server is ready, click **Run Analysis**.

### 3.3 Analysis Results

**Total Issues:** 16 *(38 incidents found)*

| # | Issue (Rule) | Incidents | File & Line(s) |
|---|---|---|---|
| 1 | Go code references `networkingv1.Ingress` type | 12 | `main.go` (multiple lines) |
| 2 | Go code references `networkingv1.IngressSpec` type | 1 | `main.go` |
| 3 | Go code references `networkingv1.IngressRule` type | 1 | `main.go` |
| 4 | Go code references `networkingv1.IngressTLS` type | 2 | `main.go` |
| 5 | Go code references `networkingv1.IngressClass` type | 2 | `main.go` |
| 6 | Go code references `networkingv1.IngressBackend` type | 1 | `main.go` |
| 7 | Go code references `networkingv1.HTTPIngressPath` type | 1 | `main.go` |
| 8 | Go code references `networkingv1.HTTPIngressRuleValue` type | 1 | `main.go` |
| 9 | Go code calls `NetworkingV1().Ingresses()` client-go API | 5 | `main.go` |
| 10 | Go code calls `NetworkingV1().IngressClasses()` client-go API | 2 | `main.go` |
| 11 | Go code sets nginx `ssl-redirect` annotation | 2 | `main.go` |
| 12 | Go code sets `configuration-snippet` annotation | 1 | `main.go` |
| 13 | Go code sets nginx proxy timeout annotations | 2 | `main.go` |
| 14 | Go code sets nginx HSTS annotations | 3 | `main.go` |
| 15 | Go code sets `kubernetes.io/ingress.class` annotation | 1 | `main.go` |
| 16 | Go code references the `ingress-nginx` controller string | 1 | `main.go` |

## Step 4: Review Violations

Click on any violation in the issues pane to see its incidents. Click on an incident to jump to the affected line in `main.go`.

### Type Reference Violations

The type reference violations (rules 1-8) detect usage of `networkingv1` types. Each violation message shows the Gateway API equivalent with before/after code.

For example, **"Go code references networkingv1.IngressRule type"** shows:

```
Replace:
  rule := networkingv1.IngressRule{
      Host: "foo.example.com",
      IngressRuleValue: networkingv1.IngressRuleValue{
          HTTP: &networkingv1.HTTPIngressRuleValue{...},
      },
  }

With an HTTPRoute:
  route := &gatewayv1.HTTPRoute{
      Spec: gatewayv1.HTTPRouteSpec{
          CommonRouteSpec: gatewayv1.CommonRouteSpec{
              ParentRefs: []gatewayv1.ParentReference{...},
          },
          Hostnames: []gatewayv1.Hostname{"foo.example.com"},
          Rules: []gatewayv1.HTTPRouteRule{...},
      },
  }
```

### Client-go API Violations

**"Go code calls NetworkingV1().Ingresses() client-go API"** shows how to replace client-go Ingress calls with the Gateway API typed client:

```
Install the Gateway API client:
  go get sigs.k8s.io/gateway-api@latest

Replace each call:
  .Ingresses(ns).Create()  -> .HTTPRoutes(ns).Create()
  .Ingresses(ns).Update()  -> .HTTPRoutes(ns).Update()
  .Ingresses(ns).Delete()  -> .HTTPRoutes(ns).Delete()
  .Ingresses(ns).Get()     -> .HTTPRoutes(ns).Get()
  .Ingresses(ns).List()    -> .HTTPRoutes(ns).List()
```

### Annotation Violations

Annotation violations show how nginx-specific annotations map to native Gateway API features. For example:

**"Go code sets nginx proxy timeout annotations"** shows:

```
Replace proxy timeout annotations with HTTPRoute timeouts:
  rule := gatewayv1.HTTPRouteRule{
      Timeouts: &gatewayv1.HTTPRouteTimeouts{
          Request:        ptr(gatewayv1.Duration("60s")),
          BackendRequest: ptr(gatewayv1.Duration("30s")),
      },
  }
```

**"Go code sets nginx HSTS annotations"** shows:

```
Replace HSTS annotations with an HTTPRoute ResponseHeaderModifier filter:
  filter := gatewayv1.HTTPRouteFilter{
      Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
      ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
          Set: []gatewayv1.HTTPHeader{
              {
                  Name:  "Strict-Transport-Security",
                  Value: "max-age=31536000; includeSubDomains",
              },
          },
      },
  }
```

## Step 5: Configure KAI (GenAI)

To generate migration fixes, configure an LLM provider.

Open the Command Palette (`Cmd+Shift+P`) and run **Konveyor: Open the GenAI model provider configuration file**. This opens `provider-settings.yaml`.

Move the `&active` YAML anchor to the provider you want to use:

**OpenAI:**
```yaml
OpenAI: &active
  environment:
    OPENAI_API_KEY: "sk-your-key-here"
  provider: "ChatOpenAI"
  args:
    model: "gpt-4o"
```

**Ollama (local, no API key):**
```yaml
ChatOllama: &active
  provider: "ChatOllama"
  args:
    model: "granite-code:8b-instruct"
    baseUrl: "127.0.0.1:11434"
```

**Amazon Bedrock:**
```yaml
AmazonBedrock: &active
  environment:
    AWS_DEFAULT_REGION: us-east-1
  provider: "ChatBedrock"
  args:
    model_id: "us.anthropic.claude-3-5-sonnet-20241022-v2:0"
```

Other supported providers include Azure OpenAI, Google Gemini, DeepSeek, and any OpenAI-compatible endpoint.

## Step 6: Generate Fixes with KAI

With a GenAI provider configured, you can generate migration fixes.

### 6.1 Fix Type References and Client-go API Calls

##### Step 1: Request a Solution

Click the wrench icon next to a violation to request a fix from KAI, or right-click on `main.go` and select **Kai-Fix All** to fix all incidents in the file.

##### Step 2: Review KAI's Solution

KAI provides a solution that replaces Ingress types with Gateway API equivalents. The changes appear in a diff editor (side-by-side view).

Key transformations KAI generates:

- `networkingv1.Ingress` → `gatewayv1.HTTPRoute`
- `networkingv1.IngressClass` → `gatewayv1.GatewayClass`
- `IngressManager` → `HTTPRouteManager` (or similar)
- `NetworkingV1().Ingresses()` → `GatewayV1().HTTPRoutes()`
- `NetworkingV1().IngressClasses()` → `GatewayV1().GatewayClasses()`
- Import changes from `k8s.io/api/networking/v1` to `sigs.k8s.io/gateway-api/apis/v1`

##### Step 3: Apply the Changes

Review the diff and click **Accept** to apply the changes.

### 6.2 Fix Annotation Violations

##### Step 1: Request Fixes for Annotation Violations

Request fixes for the remaining annotation violations. KAI replaces nginx annotations with native Gateway API features:

- `ssl-redirect` → TLS on Gateway listener + `RequestRedirect` filter
- `configuration-snippet` (headers) → `ResponseHeaderModifier` filter
- `proxy-read-timeout` / `proxy-send-timeout` → `HTTPRouteTimeouts`
- HSTS annotations → `ResponseHeaderModifier` filter with `Strict-Transport-Security` header
- `kubernetes.io/ingress.class` → `HTTPRoute.spec.parentRefs`
- `k8s.io/ingress-nginx` controller → Gateway controller name

##### Step 2: Review and Apply

Review the generated changes in the diff editor and accept them.

## Step 7: Verify the Migration

After accepting fixes:

1. The analyzer **automatically reruns** and updates the issues pane. Resolved violations disappear.

2. Install the Gateway API dependencies. The migrated code imports `sigs.k8s.io/gateway-api` which is not in the original `go.mod`:

```shell
cd go-app-v2
go get sigs.k8s.io/gateway-api@latest
go mod tidy
```

3. Run `go build` in the terminal to verify the migrated code compiles:

```shell
go build ./...
```

If there are remaining violations or build errors, repeat Step 6 to address them. The migration may require manual adjustments for struct nesting differences between Ingress and Gateway API types (e.g., `HTTPBackendRef` wraps `BackendRef` which wraps `BackendObjectReference`).

## What the Rules Cover

The 26 rules cover the full spectrum of Ingress-NGINX patterns. The sample app triggers 16 of them. The remaining 10 rules detect patterns common in other codebases:

| Pattern | Gateway API Equivalent | In Sample App |
|---|---|---|
| Ingress types (`Ingress`, `IngressSpec`, `IngressRule`, etc.) | `HTTPRoute`, `HTTPRouteSpec`, `HTTPRouteRule`, etc. | Yes |
| `NetworkingV1().Ingresses()` | `GatewayV1().HTTPRoutes()` | Yes |
| `NetworkingV1().IngressClasses()` | `GatewayV1().GatewayClasses()` | Yes |
| `ssl-redirect` annotation | TLS on Gateway listener + `RequestRedirect` filter | Yes |
| `configuration-snippet` (headers) | `ResponseHeaderModifier` filter | Yes |
| `proxy-read/send-timeout` | `HTTPRouteTimeouts` | Yes |
| HSTS annotations | `ResponseHeaderModifier` filter | Yes |
| `kubernetes.io/ingress.class` | `HTTPRoute.spec.parentRefs` | Yes |
| `k8s.io/ingress-nginx` controller | Gateway controller name | Yes |
| `rewrite-target` | `URLRewrite` filter | No |
| canary annotations | Weighted `backendRefs` | No |
| CORS annotations | Implementation-specific policy CRD | No |
| auth annotations | Implementation-specific policy CRD | No |
| rate limiting | Implementation-specific policy CRD | No |
| session affinity | Implementation-specific policy CRD | No |
| `server-snippet` | Refactor into HTTPRoute rules | No |
| `whitelist-source-range` | Implementation-specific policy CRD | No |
| `proxy-body-size` | Implementation-specific policy CRD | No |
| `proxy-http-version` (WebSocket) | Supported natively by most implementations | No |
