# Ingress-NGINX to Gateway API Migration Example

## Background

[Ingress-NGINX](https://github.com/kubernetes/ingress-nginx) is being retired. All maintenance halts in **March 2026** — after that date, no further releases, bugfixes, or security patches will be issued. The Kubernetes project recommends migrating to the [Gateway API](https://gateway-api.sigs.k8s.io/), which replaces the Ingress resource with a more expressive, role-oriented model:

| Ingress Concept | Gateway API Equivalent |
|---|---|
| `Ingress` | `HTTPRoute` (routing) + `Gateway` (listener/entrypoint) |
| `IngressClass` | `GatewayClass` |
| `kubernetes.io/ingress.class` annotation | `HTTPRoute.spec.parentRefs` |
| TLS via `spec.tls` | TLS on `Gateway.spec.listeners` |
| nginx-specific annotations | Native HTTPRoute filters or implementation-specific policies |

For full details see the [retirement announcement](https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/) and [migration guide](https://gateway-api.sigs.k8s.io/guides/getting-started/migrating-from-ingress-nginx/).

## `ingress2gateway` and Konveyor

### `ingress2gateway`

[`ingress2gateway`](https://github.com/kubernetes-sigs/ingress2gateway) is the official Kubernetes SIG-Network tool for migrating Ingress resources to Gateway API. It reads Ingress resources from a **live cluster** (via kubeconfig) or from **local YAML/JSON files** and produces equivalent Gateway and HTTPRoute resources.

The [ingress-nginx provider](https://github.com/kubernetes-sigs/ingress2gateway/tree/main/pkg/i2gw/providers/ingressnginx) handles several common nginx annotations: canary/traffic splitting, CORS, rewrites, redirects, timeouts, body size, headers, and regex paths. Annotations not yet supported (auth, rate limiting, session affinity, SSL passthrough, snippets, IP allowlisting) require manual migration.

### Where Konveyor fits in

`ingress2gateway` operates on Kubernetes resource manifests — it does not analyze source code. Many organizations have **Go programs that programmatically create and manage Ingress resources** using `k8s.io/api/networking/v1` and `client-go`. These programs build Ingress objects in code, set nginx annotations as string literals, and call the Kubernetes API to create/update them. `ingress2gateway` cannot help with this code — the Ingress resources are constructed at runtime, not stored as static YAML.

Konveyor with KAI addresses this gap:
- **Discovery** — scans Go source to find every reference to Ingress types, client-go API calls, and nginx annotation strings
- **Migration guidance** — each rule produces a violation with concrete before/after Go code examples showing the Gateway API equivalent
- **KAI fixes** — KAI uses the rule messages as LLM prompts to generate context-aware Gateway API code, understanding the surrounding Go source

## What This Consists Of

### Sample Application (`go-app/`)

A Go CLI tool (`orcapod-ingress-provisioner`) that a platform team uses to provision standardized Ingress resources for tenant applications. It exercises real-world nginx-ingress patterns:

| File | What It Does |
|---|---|
| `main.go` | Entry point — provisions ingresses for the storefront, admin dashboard, and a canary deployment |
| `ingress_manager.go` | CRUD operations via client-go, builder functions for TLS, rewrite, canary, auth, session affinity, HSTS, WebSocket, IP allowlisting, and configuration snippets |
| `ingress_helpers.go` | Config-driven builder, validation, nginx annotation utilities, ingress merging |

The app uses `k8s.io/api v0.31.0` and `k8s.io/client-go v0.31.0`.

### Konveyor Rules (`rules/`)

26 custom rules in `go-rules.yaml` — requires the Konveyor Go extension:

- **`go.referenced`** (8 rules) — detects usage of `networkingv1.Ingress`, `IngressSpec`, `IngressRule`, `IngressTLS`, `IngressClass`, `IngressBackend`, `HTTPIngressPath`, `HTTPIngressRuleValue`
- **`builtin.filecontent`** (18 rules) — detects client-go API calls (`.NetworkingV1().Ingresses(`, `.IngressClasses(`) and nginx annotation string literals in Go code (rewrite-target, ssl-redirect, configuration-snippet, server-snippet, canary, CORS, rate limits, auth, timeouts, affinity, whitelist, HSTS, proxy-body-size, WebSocket proxy-http-version, ingress class, controller reference)

Every rule includes a `message` with concrete before/after Go code examples and links to the official Gateway API migration guide. KAI uses these messages to generate migration fixes via LLM.

## How to Use

### Prerequisites

- VSCode with the [Konveyor](https://marketplace.visualstudio.com/items?itemName=konveyor.konveyor-core) and [Konveyor Go](https://marketplace.visualstudio.com/items?itemName=konveyor.konveyor-go) extensions installed
- Go toolchain (for Go extension to analyze the source)

### Steps

1. **Open the `go-app/` directory** in VSCode as the workspace root (or open the parent `nginx_migration/` directory).

2. **Add the custom rules** — go to Settings > search for `Konveyor > Analysis: Custom Rules` > click `Add Item` > enter the absolute path to the `rules/` directory.

3. **Configure analysis labels** — ensure `go` is selected as both source and target in the Konveyor analysis configuration. Rules without matching labels are filtered out.

4. **Start the Konveyor server** and **Run Analysis** from the Konveyor sidebar.

5. **Review violations** — the analyzer will flag nginx-ingress patterns in the Go source files.

6. **Use KAI** — click "Get Solution" on any violation. KAI sends the rule's message (with before/after examples) to the LLM, which generates the Gateway API equivalent code. 

