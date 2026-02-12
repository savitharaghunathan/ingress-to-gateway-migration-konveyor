# orcapod-ingress-provisioner

A Go CLI tool that provisions standardized Ingress resources for tenant applications using nginx-ingress. Each tenant gets a consistent set of Ingress resources with the company's default security headers, rate limits, and auth configuration.

This tool needs to be migrated from nginx-ingress to Gateway API before the March 2026 retirement deadline.

## Structure

| File | Description |
|---|---|
| `main.go` | Entry point — provisions ingresses for the storefront and admin dashboard |
| `ingress_manager.go` | CRUD operations via client-go, ingress builder, IngressClass management, helpers for security headers, HSTS, server-snippet, IP allowlisting, and validation |

## Nginx-Ingress Patterns Used

- **Core types**: `Ingress`, `IngressSpec`, `IngressRule`, `IngressTLS`, `IngressClass`, `IngressBackend`, `HTTPIngressPath`, `HTTPIngressRuleValue`
- **Client-go API**: `NetworkingV1().Ingresses()`, `NetworkingV1().IngressClasses()`
- **Annotations**: ssl-redirect, force-ssl-redirect, configuration-snippet, server-snippet, limit-rps, auth-url/auth-signin/auth-response-headers, proxy-read-timeout, proxy-send-timeout, proxy-body-size, affinity, session-cookie-name, session-cookie-max-age, whitelist-source-range, hsts/hsts-max-age/hsts-include-subdomains, kubernetes.io/ingress.class, k8s.io/ingress-nginx controller

## Analysis

Triggers 22 of 26 rules (52 incidents) when analyzed with the Konveyor rules in `../rules/`.
